package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"siptunnel/internal/config"
	"siptunnel/internal/nodeconfig"
	"siptunnel/internal/protocol/manscdp"
	"siptunnel/internal/protocol/siptext"
	"siptunnel/internal/service/filetransfer"
	"siptunnel/internal/tunnelmapping"
)

const (
	gbContentTypeSDP      = "application/sdp"
	gbSIPTimeout          = 20 * time.Second
	rtpChunkBytes         = 960
	catalogFragmentExpiry = 2 * time.Minute
)

var (
	globalGBMu      sync.RWMutex
	globalGBService *GB28181TunnelService
)

func SetGlobalGB28181TunnelService(svc *GB28181TunnelService) {
	globalGBMu.Lock()
	defer globalGBMu.Unlock()
	globalGBService = svc
}

func currentGB28181TunnelService() *GB28181TunnelService {
	globalGBMu.RLock()
	defer globalGBMu.RUnlock()
	return globalGBService
}

func RouteSignalPacket(ctx context.Context, remoteAddr string, payload []byte, legacy func(context.Context, []byte) ([]byte, error)) ([]byte, error) {
	if msg, err := siptext.Parse(payload); err == nil {
		if !msg.IsRequest {
			if TryHandleSIPUDPResponse(remoteAddr, payload) {
				return nil, nil
			}
			return nil, nil
		}
		svc := currentGB28181TunnelService()
		if svc == nil {
			return nil, fmt.Errorf("gb28181 tunnel service is not configured")
		}
		return svc.HandleSIP(ctx, remoteAddr, msg)
	}
	if legacy == nil {
		return nil, fmt.Errorf("no signal router accepted payload")
	}
	return legacy(ctx, payload)
}

type GB28181TunnelService struct {
	localNode          func() nodeconfig.LocalNodeConfig
	listMappings       func() []tunnelmapping.TunnelMapping
	listLocalResources func() []LocalResourceRecord
	config             func() TunnelConfigPayload
	catalog            *CatalogRegistry
	logs               *accessLogStore
	portPool           filetransfer.RTPPortPool
	protector          requestProtector

	mu               sync.Mutex
	pending          map[string]*gbPendingSession
	inbound          map[string]*gbInboundSession
	peers            map[string]*gbPeerRegistration
	onCatalogChanged func()
	subscriptions    map[string]sipDialogState
	udpRequestLanes  map[string]chan struct{}
	udpCallbackLanes map[string]chan struct{}
	catalogFragments map[string]*catalogFragmentState
	devicePenalties  map[string]*devicePenaltyState
}

type gbPendingSession struct {
	callID       string
	device       string
	mappingID    string
	responseMode string
	startedAt    time.Time
	stage        string
	lastStageAt  time.Time
	lastError    string
	dialog       sipDialogState

	responseStartCh chan manscdp.DeviceControl
	responseEndCh   chan manscdp.DeviceControl
	inlineCh        chan manscdp.DeviceControl
	byeCh           chan struct{}
	rtp             *rtpBodyReceiver
}

type closeHookReadCloser struct {
	io.ReadCloser
	once    sync.Once
	onClose func()
}

func (c *closeHookReadCloser) Close() error {
	err := c.ReadCloser.Close()
	c.once.Do(func() {
		if c.onClose != nil {
			c.onClose()
		}
	})
	return err
}

func pendingLocalRTPAddr(p *gbPendingSession) string {
	if p == nil || p.rtp == nil {
		return "-"
	}
	return net.JoinHostPort(firstNonEmpty(p.rtp.ListenIP(), "-"), strconv.Itoa(p.rtp.Port()))
}

type gbInboundSession struct {
	callID        string
	deviceID      string
	mappingID     string
	callbackAddr  string
	transport     string
	remoteRTPIP   string
	remoteRTPPort int
	localRTPIP    string
	localRTPPort  int
	rtpSender     *rtpBodySender
	startedAt     time.Time
	lastInvokeAt  time.Time
	stage         string
	lastStageAt   time.Time
	lastError     string
	mapping       tunnelmapping.TunnelMapping
	dialog        sipDialogState
}

var dialogCallIDSeq uint64

type sipDialogState struct {
	callID         string
	localURI       string
	remoteURI      string
	contactURI     string
	localTag       string
	remoteTag      string
	remoteTarget   string
	transport      string
	subscriptionID string
	nextLocalCSeq  int
	inviteCSeq     int
}

type gbPeerRegistration struct {
	deviceID              string
	remoteAddr            string
	callbackAddr          string
	transport             string
	lastRegisterAt        time.Time
	registerExpiresAt     time.Time
	lastKeepaliveAt       time.Time
	subscribedAt          time.Time
	subscriptionExpiresAt time.Time
	lastCatalogNotifyAt   time.Time
	authRequired          bool
	lastError             string
	subscriptionID        string
	subscribeDialog       sipDialogState
}

type catalogFragmentState struct {
	total     int
	order     []string
	devices   map[string]manscdp.CatalogDevice
	updatedAt time.Time
}

func NewGB28181TunnelService(localNode func() nodeconfig.LocalNodeConfig, listMappings func() []tunnelmapping.TunnelMapping, listLocalResources func() []LocalResourceRecord, cfg func() TunnelConfigPayload, catalog *CatalogRegistry, logs *accessLogStore, portPool filetransfer.RTPPortPool) *GB28181TunnelService {
	svc := &GB28181TunnelService{
		localNode:          localNode,
		listMappings:       listMappings,
		listLocalResources: listLocalResources,
		config:             cfg,
		catalog:            catalog,
		logs:               logs,
		portPool:           portPool,
		pending:            map[string]*gbPendingSession{},
		inbound:            map[string]*gbInboundSession{},
		peers:              map[string]*gbPeerRegistration{},
		subscriptions:      map[string]sipDialogState{},
		udpRequestLanes:    map[string]chan struct{}{},
		udpCallbackLanes:   map[string]chan struct{}{},
		catalogFragments:   map[string]*catalogFragmentState{},
		devicePenalties:    map[string]*devicePenaltyState{},
	}
	go svc.subscriptionLoop()
	return svc
}

// acquireUDPLane 为“同设备/同回调目标”的 UDP 并发建立可配置 lane。
//
// 设计目标不是彻底放开，而是：
// - small/control 请求允许有限并发，避免 3 路请求被硬串行；
// - bulk/RTP 长流保留独立 lane，避免大下载反压小控制面请求。
// acquireUDPLane 为同一设备/同回调目标的 UDP 并发建立 lane；对 small lane 可叠加独立等待上限。
func (s *GB28181TunnelService) acquireUDPLane(ctx context.Context, lanes map[string]chan struct{}, key string, capacity int, maxWait time.Duration) (func(), error) {
	if s == nil {
		return func() {}, nil
	}
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return func() {}, nil
	}
	if capacity <= 0 {
		capacity = 1
	}
	s.mu.Lock()
	lane, ok := lanes[key]
	if !ok || cap(lane) != capacity {
		lane = make(chan struct{}, capacity)
		lanes[key] = lane
	}
	s.mu.Unlock()
	waitCtx := ctx
	cancel := func() {}
	if maxWait > 0 {
		waitCtx, cancel = context.WithTimeout(ctx, maxWait)
	}
	defer cancel()
	select {
	case lane <- struct{}{}:
		return func() {
			select {
			case <-lane:
			default:
			}
		}, nil
	case <-waitCtx.Done():
		return nil, waitCtx.Err()
	}
}

func (s *GB28181TunnelService) acquireUDPRequestGate(ctx context.Context, remoteAddr string, mapping tunnelmapping.TunnelMapping, prepared *mappingForwardRequest) (func(), time.Duration, string, int, error) {
	if s == nil {
		return func() {}, 0, "small", 1, nil
	}
	if strings.ToUpper(strings.TrimSpace(remoteAddr)) == "" {
		return func() {}, 0, "small", 1, nil
	}
	laneClass, capacity := classifyUDPRequestLane(prepared, mapping)
	deviceID := mapping.EffectiveDeviceID()
	if penalized, delay, reason := s.currentDevicePenalty(deviceID); penalized {
		logDevicePenalty("udp_request_gate", deviceID, reason, delay)
		if err := applyPenaltyDelay(ctx, delay); err != nil {
			return nil, 0, laneClass, capacity, err
		}
		if capacity > 1 {
			capacity--
		}
	}
	key := strings.ToLower(strings.TrimSpace(remoteAddr)) + "|" + strings.ToLower(strings.TrimSpace(deviceID)) + "|" + laneClass
	started := time.Now()
	maxWait := time.Duration(0)
	if laneClass == "small" {
		maxWait = udpSmallRequestMaxWait()
	}
	release, err := s.acquireUDPLane(ctx, s.udpRequestLanes, key, capacity, maxWait)
	return release, time.Since(started), laneClass, capacity, err
}

func (s *GB28181TunnelService) acquireUDPCallbackGate(ctx context.Context, callback string, deviceID string) (func(), time.Duration, int, error) {
	if s == nil {
		return func() {}, 0, 1, nil
	}
	started := time.Now()
	capacity := udpCallbackParallelismPerPeer()
	if penalized, delay, reason := s.currentDevicePenalty(deviceID); penalized {
		logDevicePenalty("udp_callback_gate", deviceID, reason, delay)
		if err := applyPenaltyDelay(ctx, delay); err != nil {
			return nil, 0, capacity, err
		}
		if capacity > 1 {
			capacity--
		}
	}
	release, err := s.acquireUDPLane(ctx, s.udpCallbackLanes, callback, capacity, 0)
	return release, time.Since(started), capacity, err
}

func (s *GB28181TunnelService) updatePendingStage(callID, stage string, err error) {
	if s == nil || strings.TrimSpace(callID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if pending := s.pending[callID]; pending != nil {
		pending.stage = strings.TrimSpace(stage)
		pending.lastStageAt = time.Now().UTC()
		if err != nil {
			pending.lastError = err.Error()
		} else if strings.Contains(strings.ToLower(strings.TrimSpace(stage)), "ok") || strings.Contains(strings.ToLower(strings.TrimSpace(stage)), "received") || strings.Contains(strings.ToLower(strings.TrimSpace(stage)), "completed") {
			pending.lastError = ""
		}
	}
}

func (s *GB28181TunnelService) updateInboundStage(callID, stage string, err error) {
	if s == nil || strings.TrimSpace(callID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if inbound := s.inbound[callID]; inbound != nil {
		inbound.stage = strings.TrimSpace(stage)
		inbound.lastStageAt = time.Now().UTC()
		if err != nil {
			inbound.lastError = err.Error()
		} else if strings.Contains(strings.ToLower(strings.TrimSpace(stage)), "ok") || strings.Contains(strings.ToLower(strings.TrimSpace(stage)), "sent") || strings.Contains(strings.ToLower(strings.TrimSpace(stage)), "received") || strings.Contains(strings.ToLower(strings.TrimSpace(stage)), "completed") {
			inbound.lastError = ""
		}
	}
}

func (s *GB28181TunnelService) SetProtector(protector requestProtector) {
	if s == nil {
		return
	}
	s.protector = protector
}

func callbackSourceIP(addr string) string {
	if host, _, err := net.SplitHostPort(strings.TrimSpace(addr)); err == nil {
		return strings.TrimSpace(host)
	}
	return strings.TrimSpace(addr)
}

func (s *GB28181TunnelService) SetCatalogChangeCallback(fn func()) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onCatalogChanged = fn
}

func (s *GB28181TunnelService) SyncLocalCatalog() {
	if s == nil || s.catalog == nil {
		return
	}
	resources := s.catalog.Snapshot()
	log.Printf("gb28181 catalog stage=local_sync resources=%d", len(resources))
}

func (s *GB28181TunnelService) storeCatalogSubscription(state sipDialogState, expiresSec int) {
	if s == nil {
		return
	}
	key := strings.ToLower(strings.TrimSpace(firstNonEmpty(state.remoteTarget, state.remoteURI))) + ":" + strings.TrimSpace(state.subscriptionID)
	if key == ":" {
		key = strings.TrimSpace(state.callID) + ":" + strings.TrimSpace(state.subscriptionID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if expiresSec <= 0 {
		delete(s.subscriptions, key)
		return
	}
	s.subscriptions[key] = state
}

func (s *GB28181TunnelService) activeCatalogSubscriptions() []sipDialogState {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]sipDialogState, 0, len(s.subscriptions))
	for key, state := range s.subscriptions {
		if strings.TrimSpace(state.remoteTarget) == "" {
			delete(s.subscriptions, key)
			continue
		}
		out = append(out, state)
	}
	return out
}

func (s *GB28181TunnelService) TriggerCatalogPush(ctx context.Context) int {
	if s == nil {
		return 0
	}
	s.SyncLocalCatalog()
	states := s.activeCatalogSubscriptions()
	resourceCount := len(s.localCatalogDevices())
	log.Printf("gb28181 catalog stage=push_trigger subscriptions=%d resources=%d", len(states), resourceCount)
	for _, state := range states {
		s.sendCatalogNotifyInDialog(ctx, state)
	}
	return len(states)
}

func (s *GB28181TunnelService) TriggerCatalogPull(ctx context.Context) int {
	if s == nil {
		return 0
	}
	var peers []gb28181PeerView
	s.mu.Lock()
	for _, peer := range s.peers {
		if peer == nil || strings.TrimSpace(peer.callbackAddr) == "" {
			continue
		}
		peers = append(peers, gb28181PeerView{DeviceID: peer.deviceID, CallbackAddr: peer.callbackAddr, Transport: peer.transport})
	}
	s.mu.Unlock()
	for _, item := range peers {
		s.ensureCatalogSubscribe(ctx, item.CallbackAddr, item.Transport, item.DeviceID)
	}
	log.Printf("gb28181 catalog stage=pull_trigger peers=%d", len(peers))
	return len(peers)
}

func (s *GB28181TunnelService) TriggerCatalogRefresh(ctx context.Context) (int, int) {
	return s.TriggerCatalogPull(ctx), s.TriggerCatalogPush(ctx)
}

func (s *GB28181TunnelService) ExecuteForward(ctx context.Context, binding *PeerBinding, local nodeconfig.LocalNodeConfig, mapping tunnelmapping.TunnelMapping, prepared *mappingForwardRequest, transport string) (resp *http.Response, err error) {
	if prepared == nil {
		return nil, fmt.Errorf("nil prepared request")
	}
	if binding == nil || strings.TrimSpace(binding.PeerSignalingIP) == "" || binding.PeerSignalingPort <= 0 {
		return nil, fmt.Errorf("peer signaling endpoint is not configured")
	}
	remoteAddr := net.JoinHostPort(strings.TrimSpace(binding.PeerSignalingIP), strconv.Itoa(binding.PeerSignalingPort))
	callID := buildDialogCallID(prepared.MappingID, local, remoteAddr)
	tracker := newRelayTransactionTracker(callID, mapping.MappingID, mapping.EffectiveDeviceID())
	ctx = withRelayTransactionTracker(ctx, tracker)
	failureStatus := "relay_error"
	defer func() {
		if tracker == nil {
			return
		}
		if err != nil || resp == nil {
			tracker.SetFinalStatus(failureStatus)
			tracker.Finalize()
		}
	}()
	mode := normalizeResponseMode(mapping.ResponseMode)
	if mode == "" {
		mode = "AUTO"
	}
	if mode == "AUTO" && shouldPreferInlineRelay(prepared) {
		mode = "INLINE"
	}
	tracker.SetModes(mode, mode, "-")

	transport = strings.ToUpper(strings.TrimSpace(transport))
	if transport == "" {
		transport = "TCP"
	}
	var releaseRequestGate func()
	if transport == "UDP" {
		release, waited, laneClass, hardLimit, gateErr := s.acquireUDPRequestGate(ctx, remoteAddr, mapping, prepared)
		if gateErr != nil {
			failureStatus = trackerStatusError("udp_request_gate", gateErr)
			return nil, fmt.Errorf("acquire udp request gate: %w", gateErr)
		}
		releaseRequestGate = release
		tracker.SetGateWait(waited)
		tracker.SetRequestClass(laneClass)
		log.Printf("gb28181 relay stage=udp_request_gate_acquired mapping_id=%s device_id=%s remote=%s lane=%s wait_ms=%d hard_limit=%d", mapping.MappingID, mapping.EffectiveDeviceID(), remoteAddr, laneClass, waited.Milliseconds(), hardLimit)
	}
	defer func() {
		if releaseRequestGate != nil {
			releaseRequestGate()
		}
	}()
	state := newOutboundDialogState(local, remoteAddr, mapping.EffectiveDeviceID(), transport, callID)
	capability := config.DeriveCapability(local.NetworkMode)
	log.Printf("gb28181 relay stage=request_plan call_id=%s mapping_id=%s device_id=%s remote=%s transport=%s requested_mode=%s request_body_bytes=%d target_url=%s local_network_mode=%s supports_large_request_body=%t max_request_body_bytes=%d max_response_body_bytes=%d", callID, mapping.MappingID, mapping.EffectiveDeviceID(), remoteAddr, transport, mode, len(prepared.Body), prepared.TargetURL.String(), local.NetworkMode.Normalize(), capability.SupportsLargeRequestBody, mapping.MaxRequestBodyBytes, mapping.MaxResponseBodyBytes)
	negotiatedMode := mode
	var rtpReceiver *rtpBodyReceiver
	if mode != "INLINE" {
		rtpReceiver, err = newRTPBodyReceiver(local, s.portPool, callID)
		if err != nil {
			if errors.Is(err, filetransfer.ErrRTPPortExhausted) && mode == "AUTO" {
				negotiatedMode = "INLINE"
				log.Printf("gb28181 relay stage=rtp_receiver_degraded call_id=%s device_id=%s remote=%s reason=%v", callID, mapping.EffectiveDeviceID(), remoteAddr, err)
			} else {
				failureStatus = trackerStatusError("rtp_receiver_start", err)
				return nil, fmt.Errorf("start rtp receiver: %w", err)
			}
		}
	}
	if rtpReceiver != nil {
		isGenericDownload := false
		if prepared != nil {
			isGenericDownload = isGenericLargeDownloadCandidate(prepared, nil, nil)
		}
		rtpReceiver.SetTerminalCallbacks(func(stats rtpStreamStats) {
			tracker.AddRTPStats(stats)
			s.noteDeviceSuccess(mapping.EffectiveDeviceID())
		}, func(err error, stats rtpStreamStats) {
			tracker.AddRTPStats(stats)
			if !isGenericDownload {
				s.noteDeviceFailure(mapping.EffectiveDeviceID(), firstNonEmpty(classifyRecoverableRTPReadError(err), strings.TrimSpace(err.Error())))
			}
		})
		log.Printf("gb28181 relay stage=rtp_receiver_ready call_id=%s device_id=%s local_rtp=%s:%d negotiated_mode=%s", callID, mapping.EffectiveDeviceID(), rtpReceiver.ListenIP(), rtpReceiver.Port(), negotiatedMode)
	}

	cleanupDone := sync.Once{}
	cleanup := func() {
		cleanupDone.Do(func() {
			if rtpReceiver != nil {
				_ = rtpReceiver.Close()
			}
			s.removePending(callID)
		})
	}

	pending := &gbPendingSession{
		callID:          callID,
		device:          mapping.EffectiveDeviceID(),
		mappingID:       mapping.MappingID,
		responseMode:    negotiatedMode,
		startedAt:       time.Now().UTC(),
		stage:           "invite_prepare",
		lastStageAt:     time.Now().UTC(),
		dialog:          state,
		responseStartCh: make(chan manscdp.DeviceControl, 1),
		responseEndCh:   make(chan manscdp.DeviceControl, 1),
		inlineCh:        make(chan manscdp.DeviceControl, 1),
		byeCh:           make(chan struct{}, 1),
		rtp:             rtpReceiver,
	}
	s.mu.Lock()
	s.pending[callID] = pending
	s.mu.Unlock()

	cbAddr := advertisedSIPCallbackForRemote(local, remoteAddr)
	if cbAddr == "" {
		cleanup()
		failureStatus = "callback_unavailable"
		return nil, fmt.Errorf("local sip callback address is not configured for remote %s", remoteAddr)
	}
	s.updatePendingStage(callID, "invite_prepare", nil)
	logGB28181Successf("gb28181 relay stage=invite_prepare call_id=%s device_id=%s remote=%s transport=%s callback=%s mapping_id=%s", callID, mapping.EffectiveDeviceID(), remoteAddr, transport, cbAddr, mapping.MappingID)
	subject := fmt.Sprintf("%s:0,%s:0", strings.TrimSpace(local.NodeID), mapping.EffectiveDeviceID())
	invite := siptext.NewRequest("INVITE", state.remoteURI)
	fillOutboundDialogHeaders(invite, state, local, nextDialogCSeq(&state, 1, "INVITE"), "INVITE")
	invite.SetHeader("Subject", subject)
	invite.SetHeader("Content-Type", gbContentTypeSDP)
	rtpListenIP := advertisedRTPIP(local)
	rtpListenPort := 0
	if rtpReceiver != nil {
		rtpListenIP = rtpReceiver.ListenIP()
		rtpListenPort = rtpReceiver.Port()
	}
	invite.Body = manscdp.BuildRelaySDP(rtpListenIP, rtpListenPort, subject, mapping.EffectiveDeviceID(), "recvonly")
	inviteRespRaw, err := sendSIPPayload(ctx, transport, remoteAddr, invite.Bytes(), local, s.portPool, callID+"-invite")
	if err != nil {
		s.updatePendingStage(callID, "invite_error", err)
		log.Printf("gb28181 relay stage=invite_error call_id=%s device_id=%s remote=%s err=%v", callID, mapping.EffectiveDeviceID(), remoteAddr, err)
		cleanup()
		failureStatus = trackerStatusError("invite_error", err)
		return nil, fmt.Errorf("invite device %s: %w", mapping.EffectiveDeviceID(), err)
	}
	inviteResp, err := siptext.Parse(inviteRespRaw)
	if err != nil {
		cleanup()
		failureStatus = trackerStatusError("invite_parse", err)
		return nil, fmt.Errorf("parse invite response: %w", err)
	}
	if inviteResp.IsRequest || inviteResp.StatusCode/100 != 2 {
		s.updatePendingStage(callID, "invite_rejected", fmt.Errorf("status=%d", inviteResp.StatusCode))
		log.Printf("gb28181 relay stage=invite_rejected call_id=%s device_id=%s remote=%s status=%d raw=%q", callID, mapping.EffectiveDeviceID(), remoteAddr, inviteResp.StatusCode, string(inviteRespRaw))
		cleanup()
		failureStatus = fmt.Sprintf("invite_rejected:%d", inviteResp.StatusCode)
		return nil, fmt.Errorf("invite rejected: %s", string(inviteRespRaw))
	}
	pending.dialog.remoteTag = parseTagFromAddressHeader(inviteResp.Header("To"))
	if remoteTarget := parseContactAddr(inviteResp.Header("Contact")); strings.TrimSpace(remoteTarget) != "" {
		pending.dialog.remoteTarget = remoteTarget
	}
	if remoteURI := parseAddressURI(firstNonEmpty(inviteResp.Header("Contact"), inviteResp.Header("To"))); strings.TrimSpace(remoteURI) != "" {
		pending.dialog.remoteURI = remoteURI
	}
	s.updatePendingStage(callID, "invite_ok", nil)
	logGB28181Successf("gb28181 relay stage=invite_ok call_id=%s device_id=%s remote=%s status=%d", callID, mapping.EffectiveDeviceID(), remoteAddr, inviteResp.StatusCode)

	ack := siptext.NewRequest("ACK", pending.dialog.remoteURI)
	fillOutboundDialogHeaders(ack, pending.dialog, local, dialogACKCSeq(&pending.dialog, 1), "ACK")
	if err := sendSIPPayloadNoResponse(ctx, transport, firstNonEmpty(pending.dialog.remoteTarget, remoteAddr), ack.Bytes(), local, s.portPool, callID+"-ack"); err != nil {
		cleanup()
		failureStatus = trackerStatusError("ack_error", err)
		return nil, fmt.Errorf("send ack: %w", err)
	}

	requestHeaders := compactTunnelRequestHeaders(prepared.Headers, transport)
	if removed := len(prepared.Headers) - len(requestHeaders); removed > 0 {
		log.Printf("gb28181 relay stage=request_headers_compacted call_id=%s device_id=%s transport=%s original=%d compacted=%d kept_headers=%s target_url=%s", callID, mapping.EffectiveDeviceID(), transport, len(prepared.Headers), len(requestHeaders), headerKeySummary(requestHeaders), prepared.TargetURL.String())
	}
	buildInvokeMessage := func(headers http.Header) ([]byte, []byte, error) {
		invokeBody, marshalErr := manscdp.Marshal(manscdp.DeviceControl{
			CmdType:      "DeviceControl",
			TunnelStage:  "Request",
			SN:           1,
			DeviceID:     mapping.EffectiveDeviceID(),
			Method:       prepared.Method,
			RequestPath:  prepared.TargetURL.Path,
			RawQuery:     prepared.TargetURL.RawQuery,
			ResponseMode: negotiatedMode,
			Headers:      encodeXMLHeaders(headers),
			BodyBase64:   base64.StdEncoding.EncodeToString(prepared.Body),
		})
		if marshalErr != nil {
			return nil, nil, marshalErr
		}
		msgReq := siptext.NewRequest("MESSAGE", pending.dialog.remoteURI)
		fillOutboundDialogHeaders(msgReq, pending.dialog, local, nextDialogCSeq(&pending.dialog, 2, "MESSAGE"), "MESSAGE")
		msgReq.SetHeader("Content-Type", manscdp.ContentType)
		msgReq.Body = invokeBody
		return invokeBody, msgReq.Bytes(), nil
	}
	_, msgReqBytes, err := buildInvokeMessage(requestHeaders)
	if err != nil {
		cleanup()
		failureStatus = trackerStatusError("marshal_request", err)
		return nil, fmt.Errorf("marshal tunnel control xml: %w", err)
	}
	if strings.EqualFold(strings.TrimSpace(transport), "UDP") && len(msgReqBytes) > udpControlMaxBytes() {
		budgetHeaders := compactTunnelRequestHeadersForUDPBudget(prepared, requestHeaders)
		if len(budgetHeaders) != len(requestHeaders) || budgetHeaders.Get("Cookie") != requestHeaders.Get("Cookie") || budgetHeaders.Get("Content-Type") != requestHeaders.Get("Content-Type") || budgetHeaders.Get("If-None-Match") != requestHeaders.Get("If-None-Match") || budgetHeaders.Get("Range") != requestHeaders.Get("Range") {
			if _, budgetMsgBytes, budgetErr := buildInvokeMessage(budgetHeaders); budgetErr == nil {
				if len(budgetMsgBytes) < len(msgReqBytes) {
					log.Printf("gb28181 relay stage=request_control_budget_rescue call_id=%s device_id=%s remote=%s sip_bytes_before=%d sip_bytes_after=%d limit=%d method=%s path=%s headers_before=%d headers_after=%d kept_headers=%s cookie_before=%d cookie_after=%d", callID, mapping.EffectiveDeviceID(), firstNonEmpty(pending.dialog.remoteTarget, remoteAddr), len(msgReqBytes), len(budgetMsgBytes), udpControlMaxBytes(), prepared.Method, prepared.TargetURL.Path, len(requestHeaders), len(budgetHeaders), headerKeySummary(budgetHeaders), len(requestHeaders.Get("Cookie")), len(budgetHeaders.Get("Cookie")))
					requestHeaders = budgetHeaders
					msgReqBytes = budgetMsgBytes
				}
			}
		}
	}
	if strings.EqualFold(strings.TrimSpace(transport), "UDP") && len(msgReqBytes) > udpControlMaxBytes() {
		severeHeaders := compactTunnelRequestHeadersForUDPSevereBudget(prepared, requestHeaders)
		if _, severeMsgBytes, severeErr := buildInvokeMessage(severeHeaders); severeErr == nil {
			if len(severeMsgBytes) < len(msgReqBytes) {
				log.Printf("gb28181 relay stage=request_control_severe_budget_rescue call_id=%s device_id=%s remote=%s sip_bytes_before=%d sip_bytes_after=%d limit=%d method=%s path=%s headers_before=%d headers_after=%d kept_headers=%s cookie_before=%d cookie_after=%d", callID, mapping.EffectiveDeviceID(), firstNonEmpty(pending.dialog.remoteTarget, remoteAddr), len(msgReqBytes), len(severeMsgBytes), udpControlMaxBytes(), prepared.Method, prepared.TargetURL.Path, len(requestHeaders), len(severeHeaders), headerKeySummary(severeHeaders), len(requestHeaders.Get("Cookie")), len(severeHeaders.Get("Cookie")))
				requestHeaders = severeHeaders
				msgReqBytes = severeMsgBytes
			}
		}
	}
	if strings.EqualFold(strings.TrimSpace(transport), "UDP") && len(msgReqBytes) > udpControlMaxBytes() {
		suggestedLimit := minIntVal(1400, len(msgReqBytes)+32)
		log.Printf("gb28181 relay stage=request_control_oversize call_id=%s device_id=%s remote=%s sip_bytes=%d limit=%d suggested_limit=%d method=%s path=%s query_bytes=%d body_bytes=%d cookie_bytes=%d headers=%d", callID, mapping.EffectiveDeviceID(), firstNonEmpty(pending.dialog.remoteTarget, remoteAddr), len(msgReqBytes), udpControlMaxBytes(), suggestedLimit, prepared.Method, prepared.TargetURL.Path, len(prepared.TargetURL.RawQuery), len(prepared.Body), len(requestHeaders.Get("Cookie")), len(requestHeaders))
		cleanup()
		failureStatus = "udp_request_control_oversize"
		return nil, fmt.Errorf("udp request control oversize: sip_bytes=%d limit=%d method=%s path=%s", len(msgReqBytes), udpControlMaxBytes(), prepared.Method, prepared.TargetURL.Path)
	}
	infoRespRaw, err := sendSIPPayload(ctx, transport, firstNonEmpty(pending.dialog.remoteTarget, remoteAddr), msgReqBytes, local, s.portPool, callID+"-message")
	if err != nil {
		s.updatePendingStage(callID, "invoke_message_error", err)
		log.Printf("gb28181 relay stage=invoke_message_error call_id=%s device_id=%s remote=%s err=%v", callID, mapping.EffectiveDeviceID(), remoteAddr, err)
		cleanup()
		failureStatus = trackerStatusError("invoke_message_error", err)
		return nil, fmt.Errorf("send tunnel control message: %w", err)
	}
	infoResp, err := siptext.Parse(infoRespRaw)
	if err != nil {
		cleanup()
		failureStatus = trackerStatusError("invoke_message_parse", err)
		return nil, fmt.Errorf("parse message response: %w", err)
	}
	if infoResp.IsRequest || infoResp.StatusCode/100 != 2 {
		s.updatePendingStage(callID, "invoke_message_rejected", fmt.Errorf("status=%d", infoResp.StatusCode))
		log.Printf("gb28181 relay stage=invoke_message_rejected call_id=%s device_id=%s remote=%s status=%d raw=%q", callID, mapping.EffectiveDeviceID(), remoteAddr, infoResp.StatusCode, string(infoRespRaw))
		cleanup()
		failureStatus = fmt.Sprintf("invoke_message_rejected:%d", infoResp.StatusCode)
		return nil, fmt.Errorf("tunnel control message rejected: %s", string(infoRespRaw))
	}
	s.updatePendingStage(callID, "invoke_message_ok", nil)
	logGB28181Successf("gb28181 relay stage=invoke_message_ok call_id=%s device_id=%s remote=%s status=%d", callID, mapping.EffectiveDeviceID(), remoteAddr, infoResp.StatusCode)

	responseStartWait := prepared.RequestTimeout
	if responseStartWait <= 0 {
		responseStartWait = 15 * time.Second
	}
	if slack := 3 * time.Second; responseStartWait < prepared.RequestTimeout+slack {
		responseStartWait = prepared.RequestTimeout + slack
	}
	if strings.EqualFold(strings.TrimSpace(prepared.Method), http.MethodGet) && strings.EqualFold(strings.TrimSpace(prepared.Mapping.ResponseMode), "RTP") {
		minWait := 45 * time.Second
		if strings.TrimSpace(prepared.Headers.Get("Range")) != "" {
			minWait = 60 * time.Second
		}
		if responseStartWait < minWait {
			responseStartWait = minWait
		}
	}
	if penalized, delay, reason := s.currentDevicePenalty(mapping.EffectiveDeviceID()); penalized {
		responseStartWait += delay
		logDevicePenalty("response_start_wait", mapping.EffectiveDeviceID(), reason, delay)
	}
	log.Printf("gb28181 relay stage=response_start_wait call_id=%s device_id=%s remote=%s callback=%s timeout_ms=%d requested_mode=%s local_rtp=%s range_playback=%t", callID, mapping.EffectiveDeviceID(), remoteAddr, cbAddr, responseStartWait.Milliseconds(), negotiatedMode, pendingLocalRTPAddr(pending), isRangePlaybackRequest(prepared, nil, nil))
	waitCtx, cancel := context.WithTimeout(ctx, responseStartWait)
	defer cancel()
	waitStartedAt := time.Now()

	var start manscdp.DeviceControl
	select {
	case start = <-pending.responseStartCh:
	case <-waitCtx.Done():
		cleanup()
		s.noteDeviceFailure(mapping.EffectiveDeviceID(), "response_start_timeout")
		s.updatePendingStage(callID, "response_start_timeout", waitCtx.Err())
		tracker.SetResponseStartWait(time.Since(waitStartedAt))
		failureStatus = trackerStatusError("response_start_timeout", waitCtx.Err())
		log.Printf("gb28181 relay stage=response_start_timeout call_id=%s device_id=%s remote=%s callback=%s err=%v", callID, mapping.EffectiveDeviceID(), remoteAddr, cbAddr, waitCtx.Err())
		return nil, fmt.Errorf("wait response start (call_id=%s device_id=%s remote=%s callback=%s): %w", callID, mapping.EffectiveDeviceID(), remoteAddr, cbAddr, waitCtx.Err())
	}
	s.updatePendingStage(callID, "response_start_ok", nil)
	tracker.SetResponseStartWait(time.Since(waitStartedAt))
	tracker.SetModes(mode, negotiatedMode, normalizeResponseMode(start.ResponseMode))
	log.Printf("gb28181 relay stage=response_start_ok call_id=%s device_id=%s status=%d mode=%s content_length=%d", callID, mapping.EffectiveDeviceID(), start.StatusCode, normalizeResponseMode(start.ResponseMode), start.ContentLength)
	if releaseRequestGate != nil {
		releaseRequestGate()
		releaseRequestGate = nil
		log.Printf("gb28181 relay stage=udp_request_gate_released call_id=%s device_id=%s remote=%s reason=response_start_ok", callID, mapping.EffectiveDeviceID(), remoteAddr)
	}

	bodyWait := dynamicRelayBodyWait(prepared, start)
	logGB28181Successf("gb28181 relay stage=response_body_wait call_id=%s device_id=%s mode=%s content_length=%d timeout_ms=%d", callID, mapping.EffectiveDeviceID(), normalizeResponseMode(start.ResponseMode), start.ContentLength, bodyWait.Milliseconds())
	bodyCtx, bodyCancel := context.WithTimeout(ctx, bodyWait)
	cleanupWithBody := func() {
		bodyCancel()
		cleanup()
	}

	resp = &http.Response{
		StatusCode: start.StatusCode,
		Status:     fmt.Sprintf("%d %s", start.StatusCode, firstNonEmpty(start.Reason, http.StatusText(start.StatusCode))),
		Header:     decodeXMLHeaders(start.Headers),
	}
	if resp.Header == nil {
		resp.Header = make(http.Header)
	}
	modeName := normalizeResponseMode(start.ResponseMode)
	resp.Header.Set("X-Siptunnel-Response-Mode", modeName)
	resp.ContentLength = start.ContentLength
	rangePlayback := isRangePlaybackRequest(prepared, nil, resp)
	tracker.SetRequestClass(string(classifyResponseShape(prepared, resp, resp.ContentLength, mapping.MaxInlineResponseBody)))
	tracker.SetFinalStatus(trackerStatusHTTP(start.StatusCode))
	if pending.rtp != nil {
		policy := chooseRTPTolerancePolicy(prepared, nil, resp)
		pending.rtp.ConfigureTolerancePolicy(policy)
		pending.rtp.SetStreamContext(callID, mapping.EffectiveDeviceID(), resp.Header.Get("Content-Type"), rangePlayback)
	}

	switch normalizeResponseMode(start.ResponseMode) {
	case "INLINE":
		var body []byte
		select {
		case inline := <-pending.inlineCh:
			s.updatePendingStage(callID, "inline_body_received", nil)
			body, err = base64.StdEncoding.DecodeString(strings.TrimSpace(inline.BodyBase64))
			if err != nil {
				cleanupWithBody()
				failureStatus = trackerStatusError("inline_decode", err)
				return nil, fmt.Errorf("decode inline response body: %w", err)
			}
		case <-bodyCtx.Done():
			cleanupWithBody()
			failureStatus = trackerStatusError("inline_wait", bodyCtx.Err())
			return nil, fmt.Errorf("wait inline response body: %w", bodyCtx.Err())
		}
		if resp.ContentLength < 0 {
			resp.ContentLength = int64(len(body))
		}
		if resp.Header.Get("Content-Length") == "" {
			resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
		}
		resp.Body = &trackingReadCloser{ReadCloser: &closeHookReadCloser{ReadCloser: io.NopCloser(bytes.NewReader(body)), onClose: func() {
			select {
			case <-pending.byeCh:
			case <-time.After(1500 * time.Millisecond):
			}
			s.noteDeviceSuccess(mapping.EffectiveDeviceID())
			s.updatePendingStage(callID, "completed", nil)
			cleanupWithBody()
		}}, tracker: tracker}
	default:
		if pending.rtp == nil {
			cleanupWithBody()
			failureStatus = "rtp_receiver_unavailable"
			return nil, fmt.Errorf("wait rtp response body: rtp receiver unavailable")
		}
		if resp.ContentLength >= 0 && resp.Header.Get("Content-Length") == "" {
			resp.Header.Set("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
		} else if resp.ContentLength < 0 {
			resp.Header.Del("Content-Length")
		}
		resp.Body = &trackingReadCloser{ReadCloser: &closeHookReadCloser{ReadCloser: pending.rtp.Stream(bodyCtx, start.ContentLength, pending.byeCh), onClose: func() {
			s.updatePendingStage(callID, "completed", nil)
			cleanupWithBody()
		}}, tracker: tracker}
		go func() {
			select {
			case end := <-pending.responseEndCh:
				if pending.rtp != nil && end.ContentLength >= 0 {
					pending.rtp.SetExpectedBytes(end.ContentLength)
				}
				logGB28181Successf("gb28181 relay stage=response_end_ok call_id=%s device_id=%s content_length=%d", callID, mapping.EffectiveDeviceID(), end.ContentLength)
			case <-pending.byeCh:
			case <-bodyCtx.Done():
			}
		}()
	}
	return resp, nil
}

func (s *GB28181TunnelService) HandleSIP(ctx context.Context, remoteAddr string, msg *siptext.Message) ([]byte, error) {
	if msg == nil {
		return nil, fmt.Errorf("nil sip message")
	}
	if !msg.IsRequest {
		return nil, fmt.Errorf("unexpected sip response outside client transaction")
	}
	switch strings.ToUpper(strings.TrimSpace(msg.Method)) {
	case "REGISTER":
		return s.handleRegister(ctx, remoteAddr, msg)
	case "SUBSCRIBE":
		return s.handleSubscribe(ctx, remoteAddr, msg)
	case "NOTIFY":
		return s.handleNotify(msg)
	case "MESSAGE":
		return s.handleMessage(ctx, msg)
	case "INVITE":
		return s.handleInvite(msg, remoteAddr)
	case "ACK":
		s.handleACK(msg)
		return nil, nil
	case "BYE":
		return s.handleBYE(msg)
	default:
		return siptext.NewResponse(msg, 501, "Not Implemented").Bytes(), nil
	}
}

func (s *GB28181TunnelService) handleRegister(ctx context.Context, remoteAddr string, msg *siptext.Message) ([]byte, error) {
	deviceID := resolvePeerDeviceID(msg)
	callbackAddr := firstNonEmpty(parseContactAddr(msg.Header("Contact")), remoteAddr)
	transport := transportFromVia(msg.Header("Via"))
	peerKey := registrationKey(remoteAddr, deviceID)
	cfg := s.currentConfig()
	local := nodeOrZero(s.localNode)
	realm := effectiveRegisterAuthRealm(cfg, local)
	expectedUser := strings.TrimSpace(cfg.RegisterAuthUsername)
	if expectedUser == "" {
		expectedUser = strings.TrimSpace(deviceID)
	}
	if cfg.RegisterAuthEnabled {
		if !verifySIPDigestAuthorization(firstNonEmpty(msg.Header("Authorization"), msg.Header("authorization")), msg.Method, msg.RequestURI, expectedUser, strings.TrimSpace(cfg.RegisterAuthPassword), realm, strings.TrimSpace(local.NodeID)) {
			resp := siptext.NewResponse(msg, http.StatusUnauthorized, "Unauthorized")
			resp.SetHeader("WWW-Authenticate", formatSIPDigestChallenge(buildRegisterDigestChallenge(cfg, local)))
			return resp.Bytes(), nil
		}
	}
	shouldSubscribe := false
	now := time.Now().UTC()
	expiresSec := parseHeaderInt(msg.Header("Expires"), cfg.CatalogSubscribeExpiresSec)
	s.mu.Lock()
	peer := s.peers[peerKey]
	if peer == nil {
		peer = &gbPeerRegistration{deviceID: deviceID, remoteAddr: remoteAddr, callbackAddr: callbackAddr, transport: transport}
		s.peers[peerKey] = peer
	}
	peer.deviceID = firstNonEmpty(deviceID, peer.deviceID)
	peer.remoteAddr = firstNonEmpty(remoteAddr, peer.remoteAddr)
	peer.callbackAddr = firstNonEmpty(callbackAddr, peer.callbackAddr)
	peer.transport = firstNonEmpty(transport, peer.transport)
	peer.lastRegisterAt = now
	if expiresSec <= 0 {
		expiresSec = cfg.CatalogSubscribeExpiresSec
	}
	if expiresSec <= 0 {
		expiresSec = 3600
	}
	peer.registerExpiresAt = now.Add(time.Duration(expiresSec) * time.Second)
	peer.authRequired = cfg.RegisterAuthEnabled
	peer.lastError = ""
	if peer.subscriptionExpiresAt.IsZero() || now.Add(90*time.Second).After(peer.subscriptionExpiresAt) {
		shouldSubscribe = true
	}
	s.mu.Unlock()
	if shouldSubscribe {
		go s.ensureCatalogSubscribe(context.Background(), callbackAddr, transport, deviceID)
	}
	resp := siptext.NewResponse(msg, http.StatusOK, "OK")
	resp.SetHeader("Date", formatGB28181Date(now))
	resp.SetHeader("Expires", strconv.Itoa(expiresSec))
	if contact := buildLocalSIPURI(local, remoteAddr); strings.TrimSpace(contact) != "" {
		resp.SetHeader("Contact", fmt.Sprintf("<%s>", strings.Trim(contact, "<>")))
	}
	resp.SetHeader("Allow", "INVITE, ACK, CANCEL, BYE, OPTIONS, MESSAGE, SUBSCRIBE, NOTIFY")
	resp.SetHeader("Server", "SIPTunnel-Gateway/1.0")
	return resp.Bytes(), nil
}

func (s *GB28181TunnelService) handleMessage(ctx context.Context, msg *siptext.Message) ([]byte, error) {
	cmdType := manscdp.DetectCmdType(msg.Body)
	if cmdType == "Keepalive" {
		keepalive, err := manscdp.ParseKeepalive(msg.Body)
		if err == nil {
			peerKey := registrationKey("", firstNonEmpty(keepalive.DeviceID, resolvePeerDeviceID(msg)))
			s.mu.Lock()
			peer := s.peers[peerKey]
			if peer == nil {
				peer = &gbPeerRegistration{deviceID: firstNonEmpty(keepalive.DeviceID, resolvePeerDeviceID(msg))}
				s.peers[peerKey] = peer
			}
			peer.lastKeepaliveAt = time.Now().UTC()
			s.mu.Unlock()
		}
		return siptext.NewResponse(msg, http.StatusOK, "OK").Bytes(), nil
	}
	if cmdType == "DeviceControl" {
		return s.handleTunnelControlMessage(ctx, msg)
	}
	return siptext.NewResponse(msg, http.StatusOK, "OK").Bytes(), nil
}

func baseEventName(v string) string {
	trimmed := strings.TrimSpace(v)
	if idx := strings.Index(trimmed, ";"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	return strings.ToUpper(strings.TrimSpace(trimmed))
}

func eventIDFromHeader(v string) string {
	for _, part := range strings.Split(strings.TrimSpace(v), ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(strings.ToLower(part), "id=") {
			return strings.TrimSpace(part[3:])
		}
	}
	return ""
}

func buildCatalogEventHeader(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "Catalog"
	}
	return "Catalog;id=" + id
}

func isLikelyIPAddressToken(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		return false
	}
	if strings.EqualFold(v, "localhost") {
		return true
	}
	return net.ParseIP(v) != nil
}

func looksLikeDeviceID(v string) bool {
	v = strings.TrimSpace(v)
	if len(v) < 8 {
		return false
	}
	for _, r := range v {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func normalizedDialogRemoteURI(msg *siptext.Message, remoteAddr string) string {
	fromURI := parseAddressURI(msg.Header("From"))
	if fromURI != "" {
		user := parseDeviceIDFromURI(fromURI)
		if looksLikeDeviceID(user) && !isLikelyIPAddressToken(user) {
			return fromURI
		}
	}
	peerID := resolvePeerDeviceID(msg)
	if looksLikeDeviceID(peerID) {
		target := firstNonEmpty(parseContactAddr(msg.Header("Contact")), remoteAddr)
		return buildDeviceURIForRemote(peerID, target)
	}
	if fromURI != "" {
		return fromURI
	}
	return buildDeviceURIForRemote(peerID, remoteAddr)
}

func (s *GB28181TunnelService) handleSubscribe(ctx context.Context, remoteAddr string, msg *siptext.Message) ([]byte, error) {
	_ = ctx
	eventHeader := strings.TrimSpace(msg.Header("Event"))
	eventName := baseEventName(eventHeader)
	cfg := s.currentConfig()
	rawExpires := strings.TrimSpace(msg.Header("Expires"))
	expiresSec := parseHeaderInt(rawExpires, cfg.CatalogSubscribeExpiresSec)
	if rawExpires != "" {
		if parsed, err := strconv.Atoi(rawExpires); err == nil && parsed == 0 {
			expiresSec = 0
		}
	}
	if expiresSec < 0 {
		expiresSec = 0
	}
	callID := firstNonEmpty(strings.TrimSpace(msg.Header("Call-Id")), strings.TrimSpace(msg.Header("Call-ID")), fmt.Sprintf("catalog-%d", time.Now().UTC().UnixNano()))
	local := nodeOrZero(s.localNode)
	targetAddr := firstNonEmpty(parseContactAddr(msg.Header("Contact")), remoteAddr)
	transport := transportFromVia(msg.Header("Via"))
	localURI := firstNonEmpty(parseAddressURI(msg.Header("To")), buildLocalSIPURI(local, remoteAddr))
	remoteURI := normalizedDialogRemoteURI(msg, remoteAddr)
	localTag := firstNonEmpty(parseTagFromAddressHeader(msg.Header("To")), dialogTag(callID, "uas"))
	remoteTag := parseTagFromAddressHeader(msg.Header("From"))
	subscriptionID := firstNonEmpty(eventIDFromHeader(eventHeader), strconv.Itoa(parseCSeqNumber(msg.Header("CSeq"))))
	state := sipDialogState{callID: callID, localURI: localURI, remoteURI: remoteURI, contactURI: buildLocalSIPURI(local, remoteAddr), localTag: localTag, remoteTag: remoteTag, remoteTarget: targetAddr, transport: transport, subscriptionID: subscriptionID, nextLocalCSeq: parseCSeqNumber(msg.Header("CSeq")) + 1}
	resp := siptext.NewResponse(msg, http.StatusOK, "OK")
	fillInboundResponseHeaders(resp, state)
	resp.SetHeader("Expires", strconv.Itoa(expiresSec))
	if eventName != "CATALOG" {
		return resp.Bytes(), nil
	}
	s.storeCatalogSubscription(state, expiresSec)
	log.Printf("gb28181 catalog stage=subscribe_received call_id=%s remote=%s transport=%s expires=%d event=%s", callID, targetAddr, transport, expiresSec, buildCatalogEventHeader(subscriptionID))
	if expiresSec > 0 {
		go s.sendCatalogNotifyInDialog(context.Background(), state)
	}
	return resp.Bytes(), nil
}

func (s *GB28181TunnelService) handleNotify(msg *siptext.Message) ([]byte, error) {
	if baseEventName(msg.Header("Event")) == "CATALOG" || manscdp.DetectCmdType(msg.Body) == "Catalog" {
		catalog, err := manscdp.ParseCatalog(msg.Body)
		if err == nil && s.catalog != nil {
			appliedDevices, complete := s.mergeCatalogNotifyFragment(firstNonEmpty(catalog.DeviceID, resolvePeerDeviceID(msg)), catalog)
			if complete {
				s.catalog.SyncRemoteCatalog(appliedDevices, s.listMappingsSafe())
				s.mu.Lock()
				if peer := s.peers[registrationKey("", firstNonEmpty(catalog.DeviceID, resolvePeerDeviceID(msg)))]; peer != nil {
					peer.lastCatalogNotifyAt = time.Now().UTC()
					peer.lastError = ""
				}
				callback := s.onCatalogChanged
				s.mu.Unlock()
				if callback != nil {
					go callback()
				}
			}
		}
	}
	return siptext.NewResponse(msg, http.StatusOK, "OK").Bytes(), nil
}

func (s *GB28181TunnelService) ensureCatalogSubscribe(ctx context.Context, remoteAddr, transport, deviceID string) {
	remoteAddr = strings.TrimSpace(remoteAddr)
	if remoteAddr == "" {
		return
	}
	local := nodeOrZero(s.localNode)
	cfg := s.currentConfig()
	expiresSec := cfg.CatalogSubscribeExpiresSec
	if expiresSec <= 0 {
		expiresSec = 3600
	}
	peerKey := registrationKey(remoteAddr, deviceID)
	subscriptionID := ""
	s.mu.Lock()
	if peer := s.peers[peerKey]; peer != nil {
		subscriptionID = strings.TrimSpace(peer.subscriptionID)
	}
	s.mu.Unlock()
	if subscriptionID == "" {
		subscriptionID = strconv.FormatInt(time.Now().UTC().UnixNano()%1000000000, 10)
	}
	callID := buildDialogCallID("catalog-sub", local, remoteAddr)
	state := newOutboundDialogState(local, remoteAddr, firstNonEmpty(deviceID, parseDeviceIDFromAddr(remoteAddr)), transport, callID)
	state.subscriptionID = subscriptionID
	req := siptext.NewRequest("SUBSCRIBE", state.remoteURI)
	fillOutboundDialogHeaders(req, state, local, nextDialogCSeq(&state, 1, "SUBSCRIBE"), "SUBSCRIBE")
	req.SetHeader("Event", buildCatalogEventHeader(subscriptionID))
	req.SetHeader("Accept", manscdp.ContentType)
	req.SetHeader("Content-Type", manscdp.ContentType)
	req.SetHeader("Expires", strconv.Itoa(expiresSec))
	req.Body = manscdp.BuildCatalogQuery(strings.TrimSpace(deviceID), 1)
	respRaw, err := sendSIPPayload(ctx, transport, remoteAddr, req.Bytes(), local, s.portPool, callID)
	now := time.Now().UTC()
	if err != nil {
		s.mu.Lock()
		if peer := s.peers[peerKey]; peer != nil {
			peer.lastError = err.Error()
			peer.subscriptionID = subscriptionID
		}
		s.mu.Unlock()
		return
	}
	resp, err := siptext.Parse(respRaw)
	s.mu.Lock()
	if peer := s.peers[peerKey]; peer != nil {
		peer.subscriptionID = subscriptionID
		if err != nil || resp.IsRequest || resp.StatusCode/100 != 2 {
			peer.lastError = "catalog subscribe rejected"
		} else {
			state.remoteTag = parseTagFromAddressHeader(resp.Header("To"))
			if remoteURI := parseAddressURI(resp.Header("To")); remoteURI != "" {
				state.remoteURI = remoteURI
			}
			peer.subscribedAt = now
			peer.subscriptionExpiresAt = now.Add(time.Duration(expiresSec) * time.Second)
			peer.lastError = ""
			peer.subscribeDialog = state
		}
	}
	s.mu.Unlock()
	if err != nil || resp.IsRequest || resp.StatusCode/100 != 2 {
		return
	}
}

func (s *GB28181TunnelService) sendCatalogNotifyInDialog(ctx context.Context, state sipDialogState) {
	remoteAddr := strings.TrimSpace(state.remoteTarget)
	if remoteAddr == "" {
		return
	}
	local := nodeOrZero(s.localNode)
	devices := s.localCatalogDevices()
	chunks := s.buildCatalogNotifyChunks(state, local, devices)
	if len(chunks) == 0 {
		log.Printf("gb28181 catalog stage=notify_skip call_id=%s remote=%s transport=%s resources=%d reason=no_chunks", state.callID, remoteAddr, state.transport, len(devices))
		return
	}
	log.Printf("gb28181 catalog stage=notify_plan call_id=%s remote=%s transport=%s resources=%d chunks=%d max_sip_bytes_udp=%d", state.callID, remoteAddr, state.transport, len(devices), len(chunks), udpCatalogMaxBytes())
	for idx, chunk := range chunks {
		notify := siptext.NewRequest("NOTIFY", firstNonEmpty(state.remoteURI, buildDeviceURIForRemote(parseDeviceIDFromURI(state.remoteURI), remoteAddr)))
		fillOutboundDialogHeaders(notify, state, local, nextDialogCSeq(&state, 1, "NOTIFY"), "NOTIFY")
		notify.SetHeader("Event", buildCatalogEventHeader(state.subscriptionID))
		expiresSec := s.currentConfig().CatalogSubscribeExpiresSec
		if expiresSec <= 0 {
			expiresSec = 3600
		}
		notify.SetHeader("Subscription-State", fmt.Sprintf("active;expires=%d", expiresSec))
		notify.SetHeader("Content-Type", manscdp.ContentType)
		notify.Body = chunk.body
		msgBytes := notify.Bytes()
		log.Printf("gb28181 catalog stage=notify_send call_id=%s remote=%s transport=%s chunk=%d/%d resources=%d event=%s body_bytes=%d sip_bytes=%d", state.callID, remoteAddr, state.transport, idx+1, len(chunks), len(chunk.devices), buildCatalogEventHeader(state.subscriptionID), len(chunk.body), len(msgBytes))
		resp, sendErr := sendSIPPayload(ctx, state.transport, remoteAddr, msgBytes, local, s.portPool, fmt.Sprintf("%s-notify-%d", state.callID, idx+1))
		if sendErr != nil {
			log.Printf("gb28181 catalog stage=notify_send_error call_id=%s remote=%s transport=%s chunk=%d/%d err=%v", state.callID, remoteAddr, state.transport, idx+1, len(chunks), sendErr)
			return
		}
		if parsed, err := siptext.Parse(resp); err == nil && parsed != nil && !parsed.IsRequest {
			log.Printf("gb28181 catalog stage=notify_send_ok call_id=%s remote=%s transport=%s chunk=%d/%d status=%d", state.callID, remoteAddr, state.transport, idx+1, len(chunks), parsed.StatusCode)
		}
	}
}

type catalogNotifyChunk struct {
	devices []manscdp.CatalogDevice
	body    []byte
}

func (s *GB28181TunnelService) buildCatalogNotifyChunks(state sipDialogState, local nodeconfig.LocalNodeConfig, devices []manscdp.CatalogDevice) []catalogNotifyChunk {
	if len(devices) == 0 {
		body, err := manscdp.Marshal(manscdp.CatalogNotify{CmdType: "Catalog", SN: 1, DeviceID: strings.TrimSpace(local.NodeID), SumNum: 0})
		if err != nil {
			return nil
		}
		return []catalogNotifyChunk{{body: body}}
	}
	if !strings.EqualFold(strings.TrimSpace(state.transport), "UDP") {
		body, err := manscdp.Marshal(manscdp.CatalogNotify{CmdType: "Catalog", SN: 1, DeviceID: strings.TrimSpace(local.NodeID), SumNum: len(devices), DeviceList: devices})
		if err != nil {
			return nil
		}
		return []catalogNotifyChunk{{devices: append([]manscdp.CatalogDevice(nil), devices...), body: body}}
	}
	chunks := make([]catalogNotifyChunk, 0, len(devices))
	current := make([]manscdp.CatalogDevice, 0, len(devices))
	for _, dev := range devices {
		candidate := append(append([]manscdp.CatalogDevice(nil), current...), dev)
		body, sipBytes, err := s.catalogNotifyBodyAndSize(state, local, candidate, len(devices))
		if err == nil && sipBytes <= udpCatalogMaxBytes() {
			current = candidate
			continue
		}
		if len(current) > 0 {
			body, _, err := s.catalogNotifyBodyAndSize(state, local, current, len(devices))
			if err == nil {
				chunks = append(chunks, catalogNotifyChunk{devices: append([]manscdp.CatalogDevice(nil), current...), body: body})
			}
			current = nil
		}
		compact := compactCatalogDeviceForUDP(dev)
		body, sipBytes, err = s.catalogNotifyBodyAndSize(state, local, []manscdp.CatalogDevice{compact}, len(devices))
		if err != nil {
			log.Printf("gb28181 catalog stage=notify_chunk_drop call_id=%s remote=%s transport=%s device_id=%s reason=marshal_failed err=%v", state.callID, strings.TrimSpace(state.remoteTarget), state.transport, strings.TrimSpace(dev.DeviceID), err)
			continue
		}
		if sipBytes > udpCatalogMaxBytes() {
			minimal := minimalCatalogDeviceForUDP(dev)
			minBody, minSIPBytes, minErr := s.catalogNotifyBodyAndSize(state, local, []manscdp.CatalogDevice{minimal}, len(devices))
			if minErr == nil && minSIPBytes <= udpCatalogMaxBytes() {
				log.Printf("gb28181 catalog stage=notify_chunk_compacted call_id=%s remote=%s transport=%s device_id=%s original_sip_bytes=%d compacted_sip_bytes=%d", state.callID, strings.TrimSpace(state.remoteTarget), state.transport, strings.TrimSpace(dev.DeviceID), sipBytes, minSIPBytes)
				chunks = append(chunks, catalogNotifyChunk{devices: []manscdp.CatalogDevice{minimal}, body: minBody})
				continue
			}
			log.Printf("gb28181 catalog stage=notify_chunk_drop call_id=%s remote=%s transport=%s device_id=%s sip_bytes=%d limit=%d reason=single_device_oversize", state.callID, strings.TrimSpace(state.remoteTarget), state.transport, strings.TrimSpace(dev.DeviceID), sipBytes, udpCatalogMaxBytes())
			continue
		}
		chunks = append(chunks, catalogNotifyChunk{devices: []manscdp.CatalogDevice{compact}, body: body})
	}
	if len(current) > 0 {
		body, _, err := s.catalogNotifyBodyAndSize(state, local, current, len(devices))
		if err == nil {
			chunks = append(chunks, catalogNotifyChunk{devices: append([]manscdp.CatalogDevice(nil), current...), body: body})
		}
	}
	return chunks
}

func (s *GB28181TunnelService) catalogNotifyBodyAndSize(state sipDialogState, local nodeconfig.LocalNodeConfig, devices []manscdp.CatalogDevice, total int) ([]byte, int, error) {
	body, err := manscdp.Marshal(manscdp.CatalogNotify{CmdType: "Catalog", SN: 1, DeviceID: strings.TrimSpace(local.NodeID), SumNum: total, DeviceList: devices})
	if err != nil {
		return nil, 0, err
	}
	notify := siptext.NewRequest("NOTIFY", firstNonEmpty(state.remoteURI, buildDeviceURIForRemote(parseDeviceIDFromURI(state.remoteURI), strings.TrimSpace(state.remoteTarget))))
	fillOutboundDialogHeaders(notify, state, local, nextDialogCSeq(&state, 1, "NOTIFY"), "NOTIFY")
	notify.SetHeader("Event", buildCatalogEventHeader(state.subscriptionID))
	expiresSec := s.currentConfig().CatalogSubscribeExpiresSec
	if expiresSec <= 0 {
		expiresSec = 3600
	}
	notify.SetHeader("Subscription-State", fmt.Sprintf("active;expires=%d", expiresSec))
	notify.SetHeader("Content-Type", manscdp.ContentType)
	notify.Body = body
	return body, len(notify.Bytes()), nil
}

func compactCatalogDeviceForUDP(item manscdp.CatalogDevice) manscdp.CatalogDevice {
	return manscdp.CatalogDevice{
		DeviceID: strings.TrimSpace(item.DeviceID),
		Name:     strings.TrimSpace(item.Name),
		Status:   firstNonEmpty(strings.TrimSpace(item.Status), "ON"),
	}
}

func minimalCatalogDeviceForUDP(item manscdp.CatalogDevice) manscdp.CatalogDevice {
	return manscdp.CatalogDevice{
		DeviceID: strings.TrimSpace(item.DeviceID),
	}
}

func (s *GB28181TunnelService) mergeCatalogNotifyFragment(sourceDeviceID string, catalog manscdp.CatalogNotify) ([]manscdp.CatalogDevice, bool) {
	sourceDeviceID = strings.TrimSpace(sourceDeviceID)
	if sourceDeviceID == "" || s == nil {
		return catalog.DeviceList, true
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.catalogFragments == nil {
		s.catalogFragments = map[string]*catalogFragmentState{}
	}
	for key, state := range s.catalogFragments {
		if now.Sub(state.updatedAt) > catalogFragmentExpiry {
			delete(s.catalogFragments, key)
		}
	}
	total := catalog.SumNum
	if total <= 0 || total <= len(catalog.DeviceList) {
		delete(s.catalogFragments, sourceDeviceID)
		return catalog.DeviceList, true
	}
	state := s.catalogFragments[sourceDeviceID]
	if state == nil || state.total != total {
		state = &catalogFragmentState{total: total, devices: map[string]manscdp.CatalogDevice{}, order: make([]string, 0, total)}
		s.catalogFragments[sourceDeviceID] = state
	}
	state.updatedAt = now
	for _, item := range catalog.DeviceList {
		deviceID := strings.TrimSpace(item.DeviceID)
		if deviceID == "" {
			continue
		}
		if _, ok := state.devices[deviceID]; !ok {
			state.order = append(state.order, deviceID)
		}
		state.devices[deviceID] = item
	}
	collected := len(state.devices)
	log.Printf("gb28181 catalog stage=notify_fragment_received source_device=%s fragment_devices=%d collected=%d total=%d", sourceDeviceID, len(catalog.DeviceList), collected, total)
	if collected < total {
		return nil, false
	}
	merged := make([]manscdp.CatalogDevice, 0, len(state.order))
	for _, deviceID := range state.order {
		if item, ok := state.devices[deviceID]; ok {
			merged = append(merged, item)
		}
	}
	delete(s.catalogFragments, sourceDeviceID)
	log.Printf("gb28181 catalog stage=notify_fragment_complete source_device=%s total=%d", sourceDeviceID, total)
	return merged, true
}

func (s *GB28181TunnelService) localCatalogDevices() []manscdp.CatalogDevice {
	if s.catalog != nil {
		resources := s.catalog.Snapshot()
		out := make([]manscdp.CatalogDevice, 0, len(resources))
		for _, item := range resources {
			methods := allowedMethods(item.MethodList)
			out = append(out, manscdp.CatalogDevice{
				DeviceID:              strings.TrimSpace(item.DeviceID),
				Name:                  firstNonEmpty(strings.TrimSpace(item.Name), strings.TrimSpace(item.DeviceID)),
				Status:                firstNonEmpty(strings.TrimSpace(item.Status), "ON"),
				MethodList:            strings.Join(methods, ","),
				ResponseMode:          normalizedResponseMode(item.ResponseMode),
				MaxInlineResponseBody: item.MaxInlineResponseBody,
				MaxRequestBody:        item.MaxRequestBody,
			})
		}
		return out
	}
	items := s.listMappingsSafe()
	out := make([]manscdp.CatalogDevice, 0, len(items))
	for _, item := range items {
		out = append(out, manscdp.CatalogDevice{
			DeviceID: item.EffectiveDeviceID(),
			Name:     firstNonEmpty(strings.TrimSpace(item.Name), item.MappingID),
			Status:   boolStatus(item.Enabled),
		})
	}
	return out
}

func (s *GB28181TunnelService) listMappingsSafe() []tunnelmapping.TunnelMapping {
	if s.listMappings == nil {
		return nil
	}
	return s.listMappings()
}

func (s *GB28181TunnelService) handleInvite(msg *siptext.Message, remoteAddr string) ([]byte, error) {
	callID := strings.TrimSpace(msg.Header("Call-Id"))
	if callID == "" {
		callID = strings.TrimSpace(msg.Header("Call-ID"))
	}
	deviceID := firstNonEmpty(parseDeviceIDFromSubject(msg.Header("Subject")), parseDeviceIDFromURI(msg.RequestURI), parseDeviceIDFromAddressLike(msg.Header("To")), parseDeviceIDFromAddressLike(msg.Header("From")), strings.TrimSpace(msg.Header("X-Device-ID")))
	mapping, ok := s.lookupExecutionTarget(deviceID)
	if !ok {
		log.Printf("gb28181 inbound stage=invite_reject call_id=%s device_id=%s reason=resource_not_found request_uri=%s subject=%s", callID, strings.TrimSpace(deviceID), strings.TrimSpace(msg.RequestURI), strings.TrimSpace(msg.Header("Subject")))
		return siptext.NewResponse(msg, 404, "Not Found").Bytes(), nil
	}
	callbackAddr := firstNonEmpty(parseContactAddr(msg.Header("Contact")), remoteAddr)
	transport := transportFromVia(msg.Header("Via"))
	remoteIP, remotePort, _ := parseRelaySDP(msg.Body)
	if net.ParseIP(strings.TrimSpace(remoteIP)) == nil || remotePort <= 0 {
		return siptext.NewResponse(msg, 488, "Not Acceptable Here").Bytes(), nil
	}
	local := nodeOrZero(s.localNode)
	localURI := firstNonEmpty(parseAddressURI(msg.Header("To")), buildLocalSIPURI(local, remoteAddr))
	contactURI := buildLocalSIPURI(local, remoteAddr)
	remoteURI := firstNonEmpty(parseAddressURI(msg.Header("From")), buildDeviceURIForRemote(deviceID, remoteAddr))
	localTag := dialogTag(callID, "uas")
	remoteTag := parseTagFromAddressHeader(msg.Header("From"))
	state := sipDialogState{callID: callID, localURI: localURI, remoteURI: remoteURI, contactURI: contactURI, localTag: localTag, remoteTag: remoteTag, remoteTarget: callbackAddr, transport: transport}
	sender, err := newRTPBodySender(local, s.portPool, callID+"-send")
	if err != nil {
		log.Printf("gb28181 inbound stage=invite_reject call_id=%s device_id=%s reason=rtp_sender err=%v", callID, deviceID, err)
		return siptext.NewResponse(msg, 500, "Server Internal Error").Bytes(), nil
	}
	s.mu.Lock()
	s.inbound[callID] = &gbInboundSession{callID: callID, deviceID: deviceID, mappingID: mapping.MappingID, callbackAddr: callbackAddr, transport: transport, remoteRTPIP: remoteIP, remoteRTPPort: remotePort, localRTPIP: sender.ListenIP(), localRTPPort: sender.Port(), rtpSender: sender, startedAt: time.Now().UTC(), stage: "invite_received", lastStageAt: time.Now().UTC(), mapping: mapping, dialog: state}
	s.mu.Unlock()
	logGB28181Successf("gb28181 inbound stage=invite_received call_id=%s device_id=%s callback=%s transport=%s remote_rtp=%s:%d local_rtp=%s:%d mapping_id=%s", callID, deviceID, callbackAddr, transport, remoteIP, remotePort, sender.ListenIP(), sender.Port(), mapping.MappingID)

	resp := siptext.NewResponse(msg, http.StatusOK, "OK")
	fillInboundResponseHeaders(resp, state)
	resp.SetHeader("Content-Type", gbContentTypeSDP)
	resp.Body = manscdp.BuildRelaySDP(sender.ListenIP(), sender.Port(), msg.Header("Subject"), deviceID, "sendonly")
	return resp.Bytes(), nil
}

func (s *GB28181TunnelService) handleACK(msg *siptext.Message) {
	_ = msg
}

func (s *GB28181TunnelService) handleTunnelControlMessage(ctx context.Context, msg *siptext.Message) ([]byte, error) {
	control, err := manscdp.ParseDeviceControl(msg.Body)
	if err != nil {
		return siptext.NewResponse(msg, 400, "Bad Request").Bytes(), nil
	}
	callID := strings.TrimSpace(msg.Header("Call-Id"))
	if callID == "" {
		callID = strings.TrimSpace(msg.Header("Call-ID"))
	}
	resp := siptext.NewResponse(msg, http.StatusOK, "OK")
	stage := strings.ToUpper(strings.TrimSpace(control.TunnelStage))
	switch stage {
	case "REQUEST":
		s.mu.Lock()
		session := s.inbound[callID]
		s.mu.Unlock()
		if session == nil {
			return siptext.NewResponse(msg, 481, "Call/Transaction Does Not Exist").Bytes(), nil
		}
		s.mu.Lock()
		session.lastInvokeAt = time.Now().UTC()
		s.mu.Unlock()
		s.updateInboundStage(callID, "request_control_received", nil)
		logGB28181Successf("gb28181 inbound stage=request_control_received call_id=%s device_id=%s callback=%s method=%s path=%s", callID, session.deviceID, session.callbackAddr, control.Method, control.RequestPath)
		go s.executeInboundHTTP(context.Background(), session, control)
		return resp.Bytes(), nil
	case "RESPONSESTART":
		s.mu.Lock()
		pending := s.pending[callID]
		s.mu.Unlock()
		if pending != nil {
			select {
			case pending.responseStartCh <- control:
				s.updatePendingStage(callID, "response_start_received", nil)
				log.Printf("gb28181 relay stage=response_start_received call_id=%s device_id=%s status=%d mode=%s remote_mode=%s content_length=%d reason=%s", callID, pending.device, control.StatusCode, normalizeResponseMode(control.ResponseMode), strings.ToUpper(strings.TrimSpace(control.ResponseMode)), control.ContentLength, "remote_response_start")
			default:
			}
		}
		return resp.Bytes(), nil
	case "RESPONSEINLINE":
		s.mu.Lock()
		pending := s.pending[callID]
		s.mu.Unlock()
		if pending != nil {
			select {
			case pending.inlineCh <- control:
				s.updatePendingStage(callID, "inline_body_received", nil)
				logGB28181Successf("gb28181 relay stage=inline_body_received call_id=%s device_id=%s bytes_base64=%d note=%s", callID, pending.device, len(strings.TrimSpace(control.BodyBase64)), "unexpected_when_rtp_forced")
			default:
			}
		}
		return resp.Bytes(), nil
	case "RESPONSEEND":
		s.mu.Lock()
		pending := s.pending[callID]
		s.mu.Unlock()
		if pending != nil {
			if pending.rtp != nil && control.ContentLength >= 0 {
				pending.rtp.SetExpectedBytes(control.ContentLength)
			}
			select {
			case pending.responseEndCh <- control:
				logGB28181Successf("gb28181 relay stage=response_end_received call_id=%s device_id=%s content_length=%d", callID, pending.device, control.ContentLength)
			default:
			}
		}
		return resp.Bytes(), nil
	default:
		return resp.Bytes(), nil
	}
}

func (s *GB28181TunnelService) handleBYE(msg *siptext.Message) ([]byte, error) {
	callID := strings.TrimSpace(msg.Header("Call-Id"))
	if callID == "" {
		callID = strings.TrimSpace(msg.Header("Call-ID"))
	}
	s.mu.Lock()
	if pending := s.pending[callID]; pending != nil {
		select {
		case pending.byeCh <- struct{}{}:
			pending.stage = "bye_received"
			pending.lastStageAt = time.Now().UTC()
			if pending.rtp != nil {
				pending.rtp.NotifyBYE()
			}
		default:
		}
	}
	if inbound := s.inbound[callID]; inbound != nil && inbound.rtpSender != nil {
		_ = inbound.rtpSender.Close()
	}
	delete(s.inbound, callID)
	s.mu.Unlock()
	return siptext.NewResponse(msg, http.StatusOK, "OK").Bytes(), nil
}

func (s *GB28181TunnelService) executeInboundHTTP(ctx context.Context, session *gbInboundSession, invoke manscdp.DeviceControl) {
	if session == nil {
		return
	}
	mapping := session.mapping
	targetURL, err := url.Parse(buildVirtualTargetURL(mapping, invoke.RequestPath, invoke.RawQuery))
	if err != nil {
		s.updateInboundStage(session.callID, "upstream_prepare_error", err)
		s.sendFailure(session, invoke.DeviceID, http.StatusBadGateway, fmt.Sprintf("invalid target url: %v", err))
		return
	}
	body, err := base64.StdEncoding.DecodeString(strings.TrimSpace(invoke.BodyBase64))
	if err != nil {
		s.updateInboundStage(session.callID, "upstream_prepare_error", err)
		s.sendFailure(session, invoke.DeviceID, http.StatusBadGateway, fmt.Sprintf("decode body: %v", err))
		return
	}
	prepared := &mappingForwardRequest{
		MappingID:              mapping.MappingID,
		Mapping:                mapping,
		Method:                 strings.ToUpper(strings.TrimSpace(invoke.Method)),
		TargetURL:              targetURL,
		Headers:                decodeXMLHeaders(invoke.Headers),
		Body:                   body,
		ConnectTimeout:         time.Duration(mapping.ConnectTimeoutMS) * time.Millisecond,
		RequestTimeout:         time.Duration(mapping.RequestTimeoutMS) * time.Millisecond,
		ResponseHeaderTimeout:  time.Duration(mapping.ResponseTimeoutMS) * time.Millisecond,
		MaxResponseBodyBytes:   mapping.MaxResponseBodyBytes,
		AllowStreamingResponse: true,
	}
	log.Printf("gb28181 inbound stage=upstream_request call_id=%s mapping_id=%s device_id=%s callback=%s target_url=%s method=%s requested_path=%s body_bytes=%d max_request_body_bytes=%d max_response_body_bytes=%d response_mode=%s", session.callID, mapping.MappingID, session.deviceID, session.callbackAddr, targetURL.String(), prepared.Method, invoke.RequestPath, len(prepared.Body), mapping.MaxRequestBodyBytes, mapping.MaxResponseBodyBytes, normalizeResponseMode(invoke.ResponseMode))
	var release func()
	if s.protector != nil {
		permitRelease, protectErr := s.protector.Acquire(mapping.MappingID, callbackSourceIP(session.callbackAddr))
		if protectErr != nil {
			s.updateInboundStage(session.callID, "protection_reject", protectErr)
			statusCode := protectionRejectStatus(protectErr)
			if pre := classifyProtectionReject(protectErr); pre != nil {
				log.Printf("gb28181 inbound stage=protection_reject call_id=%s mapping_id=%s device_id=%s callback=%s target_url=%s method=%s requested_path=%s scope=%s kind=%s target=%s err=%v", session.callID, mapping.MappingID, session.deviceID, session.callbackAddr, targetURL.String(), prepared.Method, invoke.RequestPath, pre.Scope, pre.Kind, pre.Target, protectErr)
			} else {
				log.Printf("gb28181 inbound stage=protection_reject call_id=%s mapping_id=%s device_id=%s callback=%s target_url=%s method=%s requested_path=%s err=%v", session.callID, mapping.MappingID, session.deviceID, session.callbackAddr, targetURL.String(), prepared.Method, invoke.RequestPath, protectErr)
			}
			s.sendFailure(session, invoke.DeviceID, statusCode, "入口保护触发: "+protectErr.Error())
			return
		}
		release = permitRelease
		defer release()
	}
	upstream, err := executePreparedForward(ctx, prepared)
	if err != nil {
		s.updateInboundStage(session.callID, "upstream_failure", err)
		s.sendFailure(session, invoke.DeviceID, http.StatusBadGateway, err.Error())
		return
	}
	defer upstream.Body.Close()
	modeRequested := strings.ToUpper(strings.TrimSpace(invoke.ResponseMode))
	logRequestedResponseModeDecision(session, mapping, prepared, modeRequested)
	headerDecision := responseModeDecisionForHeaders(modeRequested, mapping, prepared, upstream, session)
	mode := headerDecision.Mode
	contentLength := headerDecision.ContentLength
	transferEncoding := strings.TrimSpace(upstream.Header.Get("Transfer-Encoding"))
	logGB28181Successf("gb28181 inbound stage=upstream_response call_id=%s device_id=%s status=%d content_type=%s content_length=%d transfer_encoding=%s mode_candidate=%s target_url=%s", session.callID, session.deviceID, upstream.StatusCode, strings.TrimSpace(upstream.Header.Get("Content-Type")), contentLength, transferEncoding, mode, targetURL.String())
	if shouldForceInlineResponse(upstream.StatusCode, contentLength, mode, session, mapping) {
		headerDecision.Mode = "INLINE"
		headerDecision.Reason = "force_inline_error_or_missing_rtp"
		mode = headerDecision.Mode
	}
	logResponseModeDecision(session, mapping, prepared, modeRequested, "effective", headerDecision)

	headers := upstream.Header.Clone()
	responseHeaders := compactTunnelResponseHeaders(headers, session.transport)
	if len(responseHeaders) == 0 {
		responseHeaders = make(http.Header)
	}
	isInternalRangeFetch := strings.EqualFold(strings.TrimSpace(prepared.Headers.Get(internalRangeFetchHeader)), "1")
	transferProfile := strings.TrimSpace(prepared.Headers.Get(downloadProfileHeader))
	if transferProfile == "" && isGenericLargeDownloadCandidate(prepared, nil, upstream) && mode != "INLINE" {
		transferProfile = "generic-rtp"
	} else if transferProfile == "" && mode != "INLINE" {
		// 设计上凡是走边界 RTP 的大响应，都应该收口到 boundary-rtp 发送 profile，
		// 这样 payload/pacing/socket-buffer/FEC 才会和 transport_tuning 的现网策略一致。
		// 之前这里留空会退回到 legacy standard(960B/3Mbps) profile，导致长视频/大文件体感吞吐显著偏低。
		transferProfile = "boundary-rtp"
	}
	if strings.EqualFold(strings.TrimSpace(session.transport), "UDP") {
		responseHeaders = compactTunnelResponseStartHeadersForUDP(responseHeaders)
		if isInternalRangeFetch {
			responseHeaders = compactInternalRangeResponseStartHeadersForUDP(responseHeaders)
		} else if mode != "INLINE" && contentLength >= boundaryFixedWindowThreshold() {
			responseHeaders = compactLargeDownloadResponseStartHeadersForUDP(responseHeaders)
		}
	}
	if contentType := strings.TrimSpace(headers.Get("Content-Type")); contentType != "" && !(strings.EqualFold(strings.TrimSpace(session.transport), "UDP") && isInternalRangeFetch) {
		responseHeaders.Set("Content-Type", contentType)
	}
	if !strings.EqualFold(strings.TrimSpace(session.transport), "UDP") {
		if contentLength >= 0 {
			responseHeaders.Set("Content-Length", strconv.FormatInt(contentLength, 10))
		}
	}
	if removed := len(headers) - len(responseHeaders); removed > 0 || isInternalRangeFetch {
		log.Printf("gb28181 inbound stage=response_headers_compacted call_id=%s device_id=%s transport=%s original=%d compacted=%d kept_headers=%s internal_range=%t profile=%s target_url=%s", session.callID, session.deviceID, session.transport, len(headers), len(responseHeaders), headerKeySummary(responseHeaders), isInternalRangeFetch, firstNonEmpty(transferProfile, "-"), targetURL.String())
	}
	var bufferedBody []byte
	if mode == "INLINE" {
		responseBody, readErr := io.ReadAll(upstream.Body)
		if readErr != nil {
			s.updateInboundStage(session.callID, "response_body_read_error", readErr)
			log.Printf("gb28181 inbound stage=response_body_read_error call_id=%s device_id=%s callback=%s err=%v", session.callID, session.deviceID, session.callbackAddr, readErr)
			if byeErr := s.sendBye(ctx, session); byeErr != nil {
				s.updateInboundStage(session.callID, "bye_send_error", byeErr)
			}
			return
		}
		if upstream.StatusCode >= 400 {
			preview := strings.TrimSpace(string(responseBody))
			if len(preview) > 160 {
				preview = preview[:160]
			}
			if preview != "" {
				log.Printf("gb28181 inbound stage=upstream_error_body call_id=%s device_id=%s status=%d target_url=%s body_preview=%q", session.callID, session.deviceID, upstream.StatusCode, targetURL.String(), preview)
			}
		}
		var finalDecision responseModeDecision
		mode, finalDecision = finalizeResponseBodyMode(mode, mapping, session, responseBody)
		logResponseModeDecision(session, mapping, prepared, modeRequested, "final", finalDecision)
		if mode != "INLINE" {
			bufferedBody = responseBody
			contentLength = int64(len(responseBody))
			if !strings.EqualFold(strings.TrimSpace(session.transport), "UDP") {
				responseHeaders.Set("Content-Length", strconv.Itoa(len(responseBody)))
			}
			log.Printf("gb28181 inbound stage=response_mode_fallback call_id=%s mapping_id=%s device_id=%s fallback=%s reason=%s body_bytes=%d max_inline_response_body=%d max_response_body_bytes=%d", session.callID, mapping.MappingID, session.deviceID, mode, "inline_payload_too_large", len(responseBody), maxOrDefault(mapping.MaxInlineResponseBody, 0), mapping.MaxResponseBodyBytes)
		} else {
			contentLength = int64(len(responseBody))
			responseHeaders.Set("Content-Length", strconv.Itoa(len(responseBody)))
			startXML, err := manscdp.Marshal(manscdp.DeviceControl{
				CmdType:       "DeviceControl",
				TunnelStage:   "ResponseStart",
				SN:            invoke.SN,
				DeviceID:      invoke.DeviceID,
				StatusCode:    upstream.StatusCode,
				Reason:        http.StatusText(upstream.StatusCode),
				ResponseMode:  mode,
				ContentLength: contentLength,
				Headers:       encodeXMLHeaders(responseHeaders),
			})
			if err == nil {
				if sendErr := s.sendInDialogRequest(ctx, session, callIDRequest("MESSAGE", session.callID, nextDialogCSeq(&session.dialog, 3, "MESSAGE")), manscdp.ContentType, startXML); sendErr != nil {
					s.updateInboundStage(session.callID, "response_start_send_error", sendErr)
					log.Printf("gb28181 inbound stage=response_start_send_error call_id=%s device_id=%s callback=%s err=%v", session.callID, session.deviceID, session.callbackAddr, sendErr)
				} else {
					s.updateInboundStage(session.callID, "response_start_sent", nil)
					log.Printf("gb28181 inbound stage=response_start_sent call_id=%s device_id=%s callback=%s status=%d mode=%s content_length=%d local_rtp=%s:%d remote_rtp=%s:%d", session.callID, session.deviceID, session.callbackAddr, upstream.StatusCode, mode, contentLength, session.localRTPIP, session.localRTPPort, session.remoteRTPIP, session.remoteRTPPort)
				}
			}
			inlineXML, err := manscdp.Marshal(manscdp.DeviceControl{
				CmdType:     "DeviceControl",
				TunnelStage: "ResponseInline",
				SN:          invoke.SN,
				DeviceID:    invoke.DeviceID,
				BodyBase64:  base64.StdEncoding.EncodeToString(responseBody),
			})
			if err == nil {
				if sendErr := s.sendInDialogRequest(ctx, session, callIDRequest("MESSAGE", session.callID, nextDialogCSeq(&session.dialog, 4, "MESSAGE")), manscdp.ContentType, inlineXML); sendErr != nil {
					s.updateInboundStage(session.callID, "inline_body_send_error", sendErr)
					log.Printf("gb28181 inbound stage=inline_body_send_error call_id=%s device_id=%s callback=%s err=%v", session.callID, session.deviceID, session.callbackAddr, sendErr)
				} else {
					s.updateInboundStage(session.callID, "inline_body_sent", nil)
					logGB28181Successf("gb28181 inbound stage=inline_body_sent call_id=%s device_id=%s callback=%s bytes=%d", session.callID, session.deviceID, session.callbackAddr, len(responseBody))
				}
			}
		}
	}

	if mode != "INLINE" {
		if session.rtpSender == nil {
			err = fmt.Errorf("rtp sender unavailable")
			s.updateInboundStage(session.callID, "rtp_body_send_error", err)
			log.Printf("gb28181 inbound stage=rtp_body_send_error call_id=%s device_id=%s remote_rtp=%s:%d err=%v", session.callID, session.deviceID, session.remoteRTPIP, session.remoteRTPPort, err)
		} else {
			startXML, err := manscdp.Marshal(manscdp.DeviceControl{
				CmdType:       "DeviceControl",
				TunnelStage:   "ResponseStart",
				SN:            invoke.SN,
				DeviceID:      invoke.DeviceID,
				StatusCode:    upstream.StatusCode,
				Reason:        http.StatusText(upstream.StatusCode),
				ResponseMode:  "RTP",
				ContentLength: contentLength,
				Headers:       encodeXMLHeaders(responseHeaders),
			})
			if err == nil {
				if sendErr := s.sendInDialogRequest(ctx, session, callIDRequest("MESSAGE", session.callID, nextDialogCSeq(&session.dialog, 3, "MESSAGE")), manscdp.ContentType, startXML); sendErr != nil {
					s.updateInboundStage(session.callID, "response_start_send_error", sendErr)
					log.Printf("gb28181 inbound stage=response_start_send_error call_id=%s device_id=%s callback=%s err=%v", session.callID, session.deviceID, session.callbackAddr, sendErr)
				} else {
					s.updateInboundStage(session.callID, "response_start_sent", nil)
					log.Printf("gb28181 inbound stage=response_start_sent call_id=%s device_id=%s callback=%s status=%d mode=%s content_length=%d local_rtp=%s:%d remote_rtp=%s:%d", session.callID, session.deviceID, session.callbackAddr, upstream.StatusCode, "RTP", contentLength, session.localRTPIP, session.localRTPPort, session.remoteRTPIP, session.remoteRTPPort)
				}
			}
			if err == nil {
				bodySource := io.Reader(upstream.Body)
				bodySourceKind := "upstream_body"
				if len(bufferedBody) > 0 {
					bodySource = bytes.NewReader(bufferedBody)
					bodySourceKind = "buffered_body"
				}
				observedBodySource := newObservedReadCloser(bodySource, nil, bodySourceKind)
				sendCtx := ctx
				if transferProfile == "generic-rtp" {
					sendCtx = withGenericDownloadContext(sendCtx, session.deviceID, targetURL.String(), strings.TrimSpace(prepared.Headers.Get(downloadTransferIDHeader)))
				}
				bodyBytes, psBytes, pesPackets, rtpPackets, sendErr := session.rtpSender.SendStreamWithProfile(sendCtx, session.remoteRTPIP, session.remoteRTPPort, observedBodySource, transferProfile)
				upstreamMetrics := observedBodySource.Snapshot()
				upstreamElapsed := upstreamMetrics.CompletedAt.Sub(upstreamMetrics.StartedAt)
				upstreamBytesPerSec := int64(0)
				upstreamBitrateBPS := int64(0)
				if upstreamElapsed > 0 {
					upstreamBytesPerSec = int64(float64(upstreamMetrics.Bytes) / upstreamElapsed.Seconds())
					upstreamBitrateBPS = upstreamBytesPerSec * 8
				}
				log.Printf("gb28181 inbound stage=upstream_body_read_summary call_id=%s device_id=%s target_url=%s source=%s upstream_body_bytes=%d upstream_read_elapsed_ms=%d upstream_body_bytes_per_sec=%d upstream_body_bitrate_bps=%d upstream_read_calls=%d upstream_read_block_ms_total=%d upstream_read_block_ms_max=%d send_err=%v", session.callID, session.deviceID, targetURL.String(), firstNonEmpty(strings.TrimSpace(upstreamMetrics.Source), bodySourceKind), upstreamMetrics.Bytes, upstreamElapsed.Milliseconds(), upstreamBytesPerSec, upstreamBitrateBPS, upstreamMetrics.Reads, upstreamMetrics.BlockedTotal.Milliseconds(), upstreamMetrics.BlockedMax.Milliseconds(), sendErr)
				if sendErr != nil {
					s.updateInboundStage(session.callID, "rtp_body_send_error", sendErr)
					log.Printf("gb28181 inbound stage=rtp_body_send_error call_id=%s device_id=%s remote_rtp=%s:%d err=%v", session.callID, session.deviceID, session.remoteRTPIP, session.remoteRTPPort, sendErr)
				} else {
					s.updateInboundStage(session.callID, "rtp_body_sent", nil)
					logGB28181Successf("gb28181 inbound stage=rtp_body_sent call_id=%s device_id=%s remote_rtp=%s:%d body_bytes=%d ps_bytes=%d pes_packets=%d rtp_packets=%d", session.callID, session.deviceID, session.remoteRTPIP, session.remoteRTPPort, bodyBytes, psBytes, pesPackets, rtpPackets)
					endXML, endErr := manscdp.Marshal(manscdp.DeviceControl{
						CmdType:       "DeviceControl",
						TunnelStage:   "ResponseEnd",
						SN:            invoke.SN,
						DeviceID:      invoke.DeviceID,
						StatusCode:    upstream.StatusCode,
						ResponseMode:  "RTP",
						ContentLength: bodyBytes,
					})
					if endErr == nil {
						if sendErr := s.sendInDialogRequest(ctx, session, callIDRequest("MESSAGE", session.callID, nextDialogCSeq(&session.dialog, 4, "MESSAGE")), manscdp.ContentType, endXML); sendErr != nil {
							log.Printf("gb28181 inbound stage=response_end_send_error call_id=%s device_id=%s callback=%s err=%v", session.callID, session.deviceID, session.callbackAddr, sendErr)
						} else {
							logGB28181Successf("gb28181 inbound stage=response_end_sent call_id=%s device_id=%s callback=%s content_length=%d", session.callID, session.deviceID, session.callbackAddr, bodyBytes)
						}
					}
				}
			}
		}
	}
	if err := s.sendBye(ctx, session); err != nil {
		s.updateInboundStage(session.callID, "bye_send_error", err)
	} else {
		s.updateInboundStage(session.callID, "completed", nil)
	}
}

func (s *GB28181TunnelService) sendFailure(session *gbInboundSession, deviceID string, statusCode int, reason string) {
	if session == nil {
		return
	}
	log.Printf("gb28181 inbound stage=upstream_failure call_id=%s device_id=%s callback=%s status=%d reason=%s", session.callID, deviceID, session.callbackAddr, statusCode, strings.TrimSpace(reason))
	startXML, err := manscdp.Marshal(manscdp.DeviceControl{
		CmdType:      "DeviceControl",
		TunnelStage:  "ResponseStart",
		SN:           1,
		DeviceID:     deviceID,
		StatusCode:   statusCode,
		Reason:       reason,
		ResponseMode: "INLINE",
	})
	if err == nil {
		if sendErr := s.sendInDialogRequest(context.Background(), session, callIDRequest("MESSAGE", session.callID, nextDialogCSeq(&session.dialog, 3, "MESSAGE")), manscdp.ContentType, startXML); sendErr != nil {
			log.Printf("gb28181 inbound stage=response_start_send_error call_id=%s device_id=%s callback=%s err=%v", session.callID, deviceID, session.callbackAddr, sendErr)
		} else {
			log.Printf("gb28181 inbound stage=response_start_sent call_id=%s device_id=%s callback=%s status=%d mode=INLINE content_length=%d", session.callID, deviceID, session.callbackAddr, statusCode, len(reason))
		}
	}
	inlineXML, err := manscdp.Marshal(manscdp.DeviceControl{
		CmdType:     "DeviceControl",
		TunnelStage: "ResponseInline",
		SN:          1,
		DeviceID:    deviceID,
		BodyBase64:  base64.StdEncoding.EncodeToString([]byte(reason)),
	})
	if err == nil {
		if sendErr := s.sendInDialogRequest(context.Background(), session, callIDRequest("MESSAGE", session.callID, nextDialogCSeq(&session.dialog, 4, "MESSAGE")), manscdp.ContentType, inlineXML); sendErr != nil {
			log.Printf("gb28181 inbound stage=inline_body_send_error call_id=%s device_id=%s callback=%s err=%v", session.callID, deviceID, session.callbackAddr, sendErr)
		} else {
			logGB28181Successf("gb28181 inbound stage=inline_body_sent call_id=%s device_id=%s callback=%s bytes=%d", session.callID, deviceID, session.callbackAddr, len(reason))
		}
	}
	if err := s.sendBye(context.Background(), session); err != nil {
		log.Printf("gb28181 inbound stage=bye_send_error call_id=%s device_id=%s callback=%s err=%v", session.callID, deviceID, session.callbackAddr, err)
	}
}

func compactUDPCallbackControlPayload(body []byte) []byte {
	control, err := manscdp.ParseDeviceControl(body)
	if err != nil {
		return body
	}
	stage := strings.ToUpper(strings.TrimSpace(control.TunnelStage))
	switch stage {
	case "RESPONSESTART":
		h := compactTunnelResponseStartHeadersForUDP(decodeXMLHeaders(control.Headers))
		control.Headers = encodeXMLHeaders(h)
	case "REQUEST":
		h := compactTunnelRequestHeaders(decodeXMLHeaders(control.Headers), "UDP")
		control.Headers = encodeXMLHeaders(h)
	case "RESPONSEEND":
		control.Headers = nil
	}
	compacted, err := manscdp.Marshal(control)
	if err != nil {
		return body
	}
	return compacted
}

func stripUDPCallbackControlHeaders(body []byte) []byte {
	control, err := manscdp.ParseDeviceControl(body)
	if err != nil {
		return body
	}
	if !strings.EqualFold(strings.TrimSpace(control.TunnelStage), "ResponseStart") {
		return body
	}
	control.Headers = nil
	compacted, err := manscdp.Marshal(control)
	if err != nil {
		return body
	}
	return compacted
}

func (s *GB28181TunnelService) sendInDialogRequest(ctx context.Context, session *gbInboundSession, req *siptext.Message, contentType string, body []byte) error {
	if session == nil {
		return fmt.Errorf("nil inbound session")
	}
	if req == nil {
		return fmt.Errorf("nil sip request")
	}
	if contentType != "" {
		req.SetHeader("Content-Type", contentType)
	}
	cseq := parseCSeqNumber(req.Header("CSeq"))
	if cseq <= 0 {
		cseq = 1
	}
	local := nodeOrZero(s.localNode)
	req.RequestURI = firstNonEmpty(session.dialog.remoteURI, buildDeviceURIForRemote(session.deviceID, session.callbackAddr))
	fillOutboundDialogHeaders(req, session.dialog, local, cseq, req.Method)
	req.Body = body
	target := firstNonEmpty(session.dialog.remoteTarget, session.callbackAddr)
	reqBytes := req.Bytes()
	if strings.EqualFold(strings.TrimSpace(session.transport), "UDP") && len(reqBytes) > udpControlMaxBytes() {
		compactedBody := compactUDPCallbackControlPayload(body)
		if !bytes.Equal(compactedBody, body) {
			req.Body = compactedBody
			reqBytes = req.Bytes()
		}
	}
	if strings.EqualFold(strings.TrimSpace(session.transport), "UDP") && len(reqBytes) > udpControlMaxBytes() {
		strippedBody := stripUDPCallbackControlHeaders(req.Body)
		if !bytes.Equal(strippedBody, req.Body) {
			req.Body = strippedBody
			reqBytes = req.Bytes()
		}
	}
	if strings.EqualFold(strings.TrimSpace(session.transport), "UDP") && len(reqBytes) > udpControlMaxBytes() {
		return fmt.Errorf("udp callback request oversize: method=%s sip_bytes=%d limit=%d callback=%s", req.Method, len(reqBytes), udpControlMaxBytes(), target)
	}
	if strings.EqualFold(strings.TrimSpace(session.transport), "UDP") {
		release, waited, hardLimit, gateErr := s.acquireUDPCallbackGate(ctx, target, session.deviceID)
		if gateErr != nil {
			return gateErr
		}
		defer release()
		log.Printf("gb28181 callback stage=udp_callback_gate_acquired method=%s call_id=%s device_id=%s callback=%s wait_ms=%d hard_limit=%d", req.Method, session.callID, session.deviceID, target, waited.Milliseconds(), hardLimit)
	}
	log.Printf("gb28181 callback stage=send method=%s call_id=%s device_id=%s callback=%s transport=%s content_type=%s body_bytes=%d sip_bytes=%d", req.Method, session.callID, session.deviceID, target, session.transport, contentType, len(body), len(reqBytes))
	resp, err := sendSIPPayload(ctx, session.transport, target, reqBytes, local, s.portPool, fmt.Sprintf("%s-%s-%d", session.callID, strings.ToLower(strings.TrimSpace(req.Method)), cseq))
	if err != nil {
		return err
	}
	if len(resp) == 0 {
		return nil
	}
	parsed, err := siptext.Parse(resp)
	if err != nil {
		return err
	}
	if parsed.IsRequest || parsed.StatusCode/100 != 2 {
		return fmt.Errorf("callback request rejected: %s", string(resp))
	}
	return nil
}

func (s *GB28181TunnelService) sendBye(ctx context.Context, session *gbInboundSession) error {
	bye := callIDRequest("BYE", session.callID, nextDialogCSeq(&session.dialog, 5, "BYE"))
	err := s.sendInDialogRequest(ctx, session, bye, "", nil)
	s.mu.Lock()
	if inbound := s.inbound[session.callID]; inbound != nil && inbound.rtpSender != nil {
		_ = inbound.rtpSender.Close()
	}
	delete(s.inbound, session.callID)
	s.mu.Unlock()
	return err
}

func (s *GB28181TunnelService) removePending(callID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pending, callID)
}

func (s *GB28181TunnelService) lookupExecutionTarget(deviceID string) (tunnelmapping.TunnelMapping, bool) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return tunnelmapping.TunnelMapping{}, false
	}
	if listFn := s.listMappings; listFn != nil {
		for _, item := range listFn() {
			if strings.EqualFold(item.EffectiveDeviceID(), deviceID) {
				return item, true
			}
		}
	}
	if listFn := s.listLocalResources; listFn != nil {
		for _, item := range listFn() {
			if !strings.EqualFold(strings.TrimSpace(item.ResourceCode), deviceID) {
				continue
			}
			if !item.Enabled {
				return tunnelmapping.TunnelMapping{}, false
			}
			if mapping, err := buildExecutionMappingFromLocalResource(item); err == nil {
				return mapping, true
			} else {
				log.Printf("gb28181 inbound stage=resource_lookup_failed device_id=%s reason=%v", deviceID, err)
			}
			return tunnelmapping.TunnelMapping{}, false
		}
	}
	return tunnelmapping.TunnelMapping{}, false
}

func buildExecutionMappingFromLocalResource(item LocalResourceRecord) (tunnelmapping.TunnelMapping, error) {
	parsed, err := url.Parse(strings.TrimSpace(item.TargetURL))
	if err != nil || parsed == nil || parsed.Host == "" {
		return tunnelmapping.TunnelMapping{}, fmt.Errorf("invalid local resource target_url: %s", strings.TrimSpace(item.TargetURL))
	}
	host := parsed.Hostname()
	if net.ParseIP(strings.TrimSpace(host)) == nil {
		return tunnelmapping.TunnelMapping{}, fmt.Errorf("local resource target host must be an IP address: %s", host)
	}
	portText := parsed.Port()
	port := 80
	if strings.EqualFold(parsed.Scheme, "https") {
		port = 443
	}
	if strings.TrimSpace(portText) != "" {
		parsedPort, convErr := strconv.Atoi(portText)
		if convErr != nil || parsedPort <= 0 || parsedPort > 65535 {
			return tunnelmapping.TunnelMapping{}, fmt.Errorf("invalid local resource target port: %s", portText)
		}
		port = parsedPort
	}
	mapping := tunnelmapping.TunnelMapping{
		MappingID:             "resource." + strings.TrimSpace(item.ResourceCode),
		DeviceID:              strings.TrimSpace(item.ResourceCode),
		Name:                  firstNonEmpty(strings.TrimSpace(item.Name), strings.TrimSpace(item.ResourceCode)),
		Enabled:               item.Enabled,
		LocalBindIP:           "127.0.0.1",
		LocalBindPort:         1,
		LocalBasePath:         "/",
		RemoteTargetIP:        host,
		RemoteTargetPort:      port,
		RemoteBasePath:        firstNonEmpty(strings.TrimSpace(parsed.EscapedPath()), "/"),
		AllowedMethods:        normalizeAllowedMethods(item.Methods),
		ResponseMode:          normalizedResponseMode(item.ResponseMode),
		ConnectTimeoutMS:      1500,
		RequestTimeoutMS:      30000,
		ResponseTimeoutMS:     30000,
		MaxInlineResponseBody: maxOrDefault(tunnelmapping.DeriveBodyLimitProfile(item.ResponseMode, false).MaxInlineResponseBody, 64*1024),
		MaxRequestBodyBytes:   maxOrDefault(tunnelmapping.DeriveBodyLimitProfile(item.ResponseMode, false).MaxRequestBodyBytes, 1<<20),
		MaxResponseBodyBytes:  maxOrDefault(tunnelmapping.DeriveBodyLimitProfile(item.ResponseMode, false).MaxResponseBodyBytes, 20<<20),
		Description:           strings.TrimSpace(item.Description),
	}
	mapping.Normalize()
	return mapping, nil
}
