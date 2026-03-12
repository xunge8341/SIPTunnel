package filetransfer

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"siptunnel/internal/protocol/rtpfile"
)

const digestSize = sha256.Size

type TransferStatus string

const (
	StatusINIT           TransferStatus = "INIT"
	StatusTRANSFERRING   TransferStatus = "TRANSFERRING"
	StatusPARTIALMISSING TransferStatus = "PARTIAL_MISSING"
	StatusRETRYING       TransferStatus = "RETRYING"
	StatusASSEMBLING     TransferStatus = "ASSEMBLING"
	StatusVERIFYING      TransferStatus = "VERIFYING"
	StatusSUCCESS        TransferStatus = "SUCCESS"
	StatusFAILED         TransferStatus = "FAILED"
)

type RetransmitRequest struct {
	TransferID    [16]byte
	MissingChunks []uint32
	RequestedAt   time.Time
}

type ChunkRecord struct {
	ChunkNo     uint32
	ChunkOffset uint64
	ChunkLength uint32
	ChunkDigest [digestSize]byte
}

type ReceiveResult struct {
	TransferID   [16]byte
	Status       TransferStatus
	Completed    bool
	Missing      []uint32
	Retransmit   *RetransmitRequest
	Duplicate    bool
	TempFilePath string
}

type Receiver struct {
	mu    sync.Mutex
	tasks map[[16]byte]*TransferTask
	dir   string
}

func NewReceiver(dir string) *Receiver {
	return &Receiver{tasks: make(map[[16]byte]*TransferTask), dir: dir}
}

func (r *Receiver) AddChunk(packet rtpfile.ChunkPacket) (*ReceiveResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	transferID := packet.Header.TransferID
	task, ok := r.tasks[transferID]
	if !ok {
		created, err := newTransferTask(r.dir, packet)
		if err != nil {
			return nil, err
		}
		task = created
		r.tasks[transferID] = task
	}

	res, err := task.AddChunk(packet)
	if err != nil {
		return nil, err
	}
	if res.Completed {
		delete(r.tasks, transferID)
	}
	return res, nil
}

func (r *Receiver) DetectMissing(transferID [16]byte) (*ReceiveResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	task, ok := r.tasks[transferID]
	if !ok {
		return nil, fmt.Errorf("transfer %x not found", transferID)
	}
	return task.DetectMissing(), nil
}

func (r *Receiver) MarkRetrying(transferID [16]byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[transferID]
	if !ok {
		return fmt.Errorf("transfer %x not found", transferID)
	}
	task.MarkRetrying()
	return nil
}

type TransferTask struct {
	id         [16]byte
	status     TransferStatus
	fileDigest [digestSize]byte
	fileSize   uint64
	totalChunk uint32
	tracker    *chunkTracker
	store      *tempFileStore
}

func newTransferTask(dir string, packet rtpfile.ChunkPacket) (*TransferTask, error) {
	h := packet.Header
	if h.ChunkTotal == 0 {
		return nil, errors.New("chunk_total must be positive")
	}
	if h.FileSize == 0 {
		return nil, errors.New("file_size must be positive")
	}
	store, err := newTempFileStore(dir, h.TransferID, h.FileSize)
	if err != nil {
		return nil, err
	}
	return &TransferTask{
		id:         h.TransferID,
		status:     StatusINIT,
		fileDigest: h.FileDigest,
		fileSize:   h.FileSize,
		totalChunk: h.ChunkTotal,
		tracker:    newChunkTracker(h.ChunkTotal),
		store:      store,
	}, nil
}

func (t *TransferTask) AddChunk(packet rtpfile.ChunkPacket) (*ReceiveResult, error) {
	h := packet.Header
	if err := t.validatePacket(packet); err != nil {
		t.status = StatusFAILED
		return nil, err
	}

	duplicate, err := t.tracker.Track(ChunkRecord{
		ChunkNo:     h.ChunkNo,
		ChunkOffset: h.ChunkOffset,
		ChunkLength: h.ChunkLength,
		ChunkDigest: h.ChunkDigest,
	})
	if err != nil {
		t.status = StatusFAILED
		return nil, err
	}
	if duplicate {
		return &ReceiveResult{TransferID: t.id, Status: t.status, Duplicate: true, TempFilePath: t.store.Path()}, nil
	}

	if err := t.store.WriteChunk(h.ChunkOffset, packet.Payload); err != nil {
		t.status = StatusFAILED
		return nil, err
	}
	if t.status == StatusINIT || t.status == StatusPARTIALMISSING || t.status == StatusRETRYING {
		t.status = StatusTRANSFERRING
	}

	if !t.tracker.Complete() {
		return &ReceiveResult{TransferID: t.id, Status: t.status, TempFilePath: t.store.Path()}, nil
	}

	t.status = StatusASSEMBLING
	t.status = StatusVERIFYING
	if err := t.store.VerifyDigest(t.fileDigest, t.fileSize); err != nil {
		t.status = StatusFAILED
		return &ReceiveResult{TransferID: t.id, Status: t.status, Completed: true, TempFilePath: t.store.Path()}, fmt.Errorf("file digest verification failed: %w", err)
	}
	t.status = StatusSUCCESS
	return &ReceiveResult{TransferID: t.id, Status: t.status, Completed: true, TempFilePath: t.store.Path()}, nil
}

func (t *TransferTask) DetectMissing() *ReceiveResult {
	missing := t.tracker.MissingChunks()
	res := &ReceiveResult{TransferID: t.id, Status: t.status, Missing: missing, TempFilePath: t.store.Path()}
	if len(missing) == 0 {
		return res
	}
	t.status = StatusPARTIALMISSING
	res.Status = t.status
	res.Retransmit = &RetransmitRequest{TransferID: t.id, MissingChunks: append([]uint32(nil), missing...), RequestedAt: time.Now()}
	return res
}

func (t *TransferTask) MarkRetrying() {
	t.status = StatusRETRYING
}

func (t *TransferTask) validatePacket(packet rtpfile.ChunkPacket) error {
	h := packet.Header
	if h.TransferID != t.id {
		return errors.New("transfer_id mismatch")
	}
	if h.ChunkTotal != t.totalChunk {
		return fmt.Errorf("chunk_total mismatch: got=%d want=%d", h.ChunkTotal, t.totalChunk)
	}
	if h.FileSize != t.fileSize {
		return fmt.Errorf("file_size mismatch: got=%d want=%d", h.FileSize, t.fileSize)
	}
	if h.FileDigest != t.fileDigest {
		return errors.New("file_digest mismatch in header")
	}
	if h.ChunkNo == 0 || h.ChunkNo > t.totalChunk {
		return fmt.Errorf("invalid chunk_no=%d", h.ChunkNo)
	}
	if uint32(len(packet.Payload)) != h.ChunkLength {
		return fmt.Errorf("chunk_length mismatch: got=%d want=%d", len(packet.Payload), h.ChunkLength)
	}
	if h.ChunkOffset+uint64(h.ChunkLength) > t.fileSize {
		return fmt.Errorf("chunk out of file range offset=%d length=%d file=%d", h.ChunkOffset, h.ChunkLength, t.fileSize)
	}
	if sha256.Sum256(packet.Payload) != h.ChunkDigest {
		return fmt.Errorf("chunk_digest mismatch chunk=%d", h.ChunkNo)
	}
	return nil
}

func (t *TransferTask) ReceivedBitmap() []bool {
	return t.tracker.Bitmap()
}

func (t *TransferTask) ReceivedChunkMap() map[uint32]ChunkRecord {
	return t.tracker.ChunkMap()
}

type chunkTracker struct {
	total    uint32
	received map[uint32]ChunkRecord
	bitmap   []bool
}

func newChunkTracker(total uint32) *chunkTracker {
	return &chunkTracker{total: total, received: make(map[uint32]ChunkRecord, total), bitmap: make([]bool, total)}
}

func (c *chunkTracker) Track(rec ChunkRecord) (duplicate bool, err error) {
	if rec.ChunkNo == 0 || rec.ChunkNo > c.total {
		return false, fmt.Errorf("chunk_no out of range: %d", rec.ChunkNo)
	}
	if existing, ok := c.received[rec.ChunkNo]; ok {
		if existing != rec {
			return false, fmt.Errorf("duplicate chunk metadata mismatch chunk=%d", rec.ChunkNo)
		}
		return true, nil
	}
	c.received[rec.ChunkNo] = rec
	c.bitmap[rec.ChunkNo-1] = true
	return false, nil
}

func (c *chunkTracker) Complete() bool {
	return len(c.received) == int(c.total)
}

func (c *chunkTracker) MissingChunks() []uint32 {
	missing := make([]uint32, 0)
	for i := uint32(1); i <= c.total; i++ {
		if !c.bitmap[i-1] {
			missing = append(missing, i)
		}
	}
	return missing
}

func (c *chunkTracker) Bitmap() []bool {
	out := make([]bool, len(c.bitmap))
	copy(out, c.bitmap)
	return out
}

func (c *chunkTracker) ChunkMap() map[uint32]ChunkRecord {
	out := make(map[uint32]ChunkRecord, len(c.received))
	for k, v := range c.received {
		out[k] = v
	}
	return out
}

type tempFileStore struct {
	file *os.File
	size uint64
}

func newTempFileStore(dir string, transferID [16]byte, size uint64) (*tempFileStore, error) {
	if dir == "" {
		dir = os.TempDir()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	name := fmt.Sprintf("rtp-transfer-%x-*.part", transferID)
	f, err := os.CreateTemp(dir, name)
	if err != nil {
		return nil, err
	}
	if err := f.Truncate(int64(size)); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return nil, err
	}
	return &tempFileStore{file: f, size: size}, nil
}

func (s *tempFileStore) WriteChunk(offset uint64, payload []byte) error {
	if offset+uint64(len(payload)) > s.size {
		return fmt.Errorf("write exceeds file size offset=%d len=%d size=%d", offset, len(payload), s.size)
	}
	if _, err := s.file.WriteAt(payload, int64(offset)); err != nil {
		return err
	}
	return nil
}

func (s *tempFileStore) VerifyDigest(expected [digestSize]byte, expectedSize uint64) error {
	if expectedSize != s.size {
		return fmt.Errorf("unexpected file size: got=%d want=%d", s.size, expectedSize)
	}
	if _, err := s.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	h := sha256.New()
	if _, err := io.CopyN(h, s.file, int64(s.size)); err != nil {
		return err
	}
	var actual [digestSize]byte
	copy(actual[:], h.Sum(nil))
	if actual != expected {
		return fmt.Errorf("digest mismatch")
	}
	return nil
}

func (s *tempFileStore) Path() string {
	if s == nil || s.file == nil {
		return ""
	}
	path, err := filepath.Abs(s.file.Name())
	if err != nil {
		return s.file.Name()
	}
	return path
}
