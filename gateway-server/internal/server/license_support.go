package server

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const defaultLicensePrivateKeyPEM = `-----BEGIN PRIVATE KEY-----
MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQCr3qj6metQ+c1W
89/VQqWy2Dn6Jqs6sZE0Wl8ySg8TL8L60BbWWQI97kDndWhAlnXFFg6sNdONyaxx
8d5YGAelIppXbRok9t/pilEdLnidSCsikeO9YGCYqzBqD48mQZtlVp18N1Kt0xR5
y9zzJGvmkYuGB8X4AnVNhI7cy7XnVsPsnCIYtdwO04t01S4NwxQD8FYRKIw5DvjO
rY1eoQCgHtUouOifAmbsSvXTYrLXvH2SlB0oTesUng3cBOxSk2Fzj6AOZhHaBQQO
kfHyiobO/VVIezS9ng7OvqLFWwwJX21x0og1WNNf5p6Na1IvDKikotLPOPOEU/xQ
eHz6v2+1AgMBAAECggEAJO4JHR/polKqvp5UYDyb4hv4CTo53Li+3KL2hZXIO3Ut
zjrcTV5+ztPc+l8N8aLi12Bv8qz2Mic9eJZiEkfHjMIsq9Bzp7GsV0AzQbP0kscp
DZqPdue4mdVe58tEFRJP06yS1lgE2fjbE3isl0oiLT7k3f3ZmfrdPbTYBbV39qch
CTULhjNLzh4ozejJOG7al2kUzdzcSYHMhJh2EqK/psOl947rBSC62JA2oeAV9hBv
q+k1nlLLIKSHqKTHCBoTmzULZoHSzOPABYf9sls2rakggcXZHBiuNZwkMCNMy0vj
hW4fywYZOfEu5Kvc+1DhJtOfkmujoPw3d5rf8g7HAQKBgQDySvzemB2H1BoFXVX5
ISgoZlsjdq5rMcKw+wIsIUyAb5wb0qk84Qwi2sKuFxnPhNUP4oZ+220zsMTiH/oK
YR1GSI1ACPwfOtfYWwnz6HtLAhKJkl634x3U9WnF1V+HWrMZKzZ2YqPa9uWdcEoS
pswk1qyILAEFFOCtLXTIjPna1QKBgQC1l8R7QOqbrzfHrmRaiHj6sxOct3X7PtDX
1efnkG48wN3TM5PJe4DUGOTTwBbcmETNRI/A/qnJ6CBtjvM4GMf0UFZswTgvgl7b
LisJqu3fwuLB6iJArmbg6S1q8tPoL0K6tcORrMl409qhh67MSnH0MaSWOd2wUuAf
LuXA603xYQKBgQDeDxvyZjeqZRn0ELbavSiw3h5pQjxYwiJNUb+L8njKvX+1gDzb
LuaQiy4hn8poBrW++T2KxlAvL7NCC0x+dsL9x0Ctj46CkMuB3u4gPNHCzQNwUlW8
8spEgyeNySDkTJwYVSJ1HbJO3DlVMbSxo2011goKQ0or/hZsoVyG8a2MgQKBgFmX
rVraJmX1RuH/yodYOcgGvjBd25m/3i3+3VHEUn8q8MaY9ds8Uc1TEuLeLOldPuS/
ZOVlP8PcANPM6XbN0ylY0asKkXvvKHmfB6DXclEpx9LAf3HGGf/xS3UupRoy5wtT
Tk/7HdO9QmrblIQ6XoqKS5fKqPOrj+QSsUxDS8tBAoGAN1fRK9az2ChOG21n3nbO
z83L8rrYDK5GXZZnRcFu3EWmElzGVWVvjvgl1/hV7cbukCIFmjVSMeuZjrBC1BXu
fpv9cDMicnqaEURGX9o3eNtSXJO86kOzt6C9INJHvzur/SJhZdn+fs25y21tnjbH
a4Q6tAUTP2dvLiaF8sirZSs=
-----END PRIVATE KEY-----`

var licenseAssembleOrder = []string{
	"Machine Code",
	"Product Type",
	"License Time",
	"Active Time",
	"Expire Time",
	"Maintenance Expire Time",
	"License Type",
	"License Counter",
	"OA Request No",
	"Server Info",
	"Project Code",
	"Region Info",
	"Industry Info",
	"Customer Info",
	"User Info",
	"Extend Info",
}

type licenseSections map[string]map[string]string

type licenseHardwareInfo struct {
	Hostname    string `json:"hostname"`
	NodeID      string `json:"node_id"`
	CPUID       string `json:"cpu_id"`
	BoardSerial string `json:"board_serial"`
	MACAddress  string `json:"mac_address"`
	MachineCode string `json:"machine_code"`
	RequestFile string `json:"request_file"`
}

func productTypeDisplayName(code string) string {
	switch strings.TrimSpace(code) {
	case "6":
		return "SIP隧道网关"
	default:
		return "未知产品"
	}
}

func collectLicenseHardware(nodeID string) licenseHardwareInfo {
	hostname, _ := os.Hostname()
	cpuID := hardwareFirstNonEmpty(probeCPUId(), "CPU-DEFAULT")
	boardSerial := hardwareFirstNonEmpty(probeBoardSerial(), "BOARD-DEFAULT")
	mac := hardwareFirstNonEmpty(probePrimaryMAC(), "MAC-DEFAULT")
	machineCode := buildMachineCode(cpuID, boardSerial, mac)
	hw := licenseHardwareInfo{
		Hostname:    hardwareFirstNonEmpty(hostname, "unknown-host"),
		NodeID:      hardwareFirstNonEmpty(nodeID, "unknown-node"),
		CPUID:       cpuID,
		BoardSerial: boardSerial,
		MACAddress:  mac,
		MachineCode: machineCode,
	}
	hw.RequestFile = buildLicenseRequestFile(hw)
	return hw
}

func hardwareFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func buildMachineCode(cpuID, boardSerial, mac string) string {
	raw := strings.ToUpper(strings.Join([]string{cpuID, boardSerial, mac}, "|"))
	sum := md5.Sum([]byte(raw))
	n := binary.BigEndian.Uint64(sum[:8]) % 1000000000000
	return fmt.Sprintf("%012d", n)
}

func buildLicenseRequestFile(hw licenseHardwareInfo) string {
	var b strings.Builder
	b.WriteString("[Description Info]\n")
	b.WriteString("Description=\"SIP Tunnel license request file\"\n\n")
	b.WriteString("[License Info]\n")
	b.WriteString(fmt.Sprintf("Machine Code=%s\n", hw.MachineCode))
	b.WriteString(fmt.Sprintf("Server Info=%s|%s\n", hw.Hostname, hw.NodeID))
	b.WriteString("Project Code=\n")
	b.WriteString("Region Info=\n")
	b.WriteString("Industry Info=\n")
	b.WriteString("Customer Info=\n")
	b.WriteString("User Info=\n")
	b.WriteString("Extend Info={}\n\n")
	b.WriteString("[Hardware Info]\n")
	b.WriteString(fmt.Sprintf("CPUID=%s\n", hw.CPUID))
	b.WriteString(fmt.Sprintf("Board Serial=%s\n", hw.BoardSerial))
	b.WriteString(fmt.Sprintf("MAC Address=%s\n", hw.MACAddress))
	return b.String()
}

func buildTrialLicenseContent(hw licenseHardwareInfo, now time.Time) (string, LicenseInfoPayload, error) {
	now = now.UTC()
	active := now.Format("2006-01-02")
	expire := now.Add(7 * 24 * time.Hour).Format("2006-01-02")
	maintenance := now.Add(6 * 24 * time.Hour).Format("2006-01-02")
	sections := licenseSections{
		"Description Info": {"Description": "SIP Tunnel license file"},
		"License Info": {
			"Machine Code":            hw.MachineCode,
			"Product Type":            "6",
			"License Time":            active,
			"Active Time":             active,
			"Expire Time":             expire,
			"Maintenance Expire Time": maintenance,
			"License Type":            "1",
			"License Counter":         "1",
			"OA Request No":           "AUTO-TRIAL",
			"Server Info":             fmt.Sprintf("%s|%s", hw.Hostname, hw.NodeID),
			"Project Code":            "自动试用授权",
			"Region Info":             "000000000000",
			"Industry Info":           "trial",
			"Customer Info":           hw.Hostname,
			"User Info":               hw.NodeID,
			"Extend Info":             `{"feature":"trial"}`,
		},
		"Summary Info": {},
	}
	md5Value := fmt.Sprintf("%x", md5.Sum([]byte(assembleLicenseValues(sections))))
	summary1, err := encryptSummary1(md5Value)
	if err != nil {
		return "", defaultLicenseInfo(), err
	}
	sections["Summary Info"]["Summary1"] = summary1
	sections["Summary Info"]["Summary2"] = ""
	content := renderLicenseINI(sections)
	info, err := verifyLicenseSummary(content, hw.MachineCode)
	if err != nil {
		return "", defaultLicenseInfo(), err
	}
	info.LastVerifyResult = "自动生成 7 天试用授权"
	return content, info, nil
}

func encryptSummary1(md5Value string) (string, error) {
	privateKey, err := currentLicensePrivateKey()
	if err != nil {
		return "", err
	}
	pub := &privateKey.PublicKey
	encoded := base64.StdEncoding.EncodeToString([]byte(strings.TrimSpace(md5Value)))
	cipherText, err := rsa.EncryptPKCS1v15(rand.Reader, pub, []byte(encoded))
	if err != nil {
		return "", fmt.Errorf("summary1 rsa encrypt failed: %w", err)
	}
	return base64.StdEncoding.EncodeToString(cipherText), nil
}

func renderLicenseINI(sections licenseSections) string {
	orderedSections := []string{"Description Info", "License Info", "Summary Info"}
	sectionOrder := map[string][]string{
		"Description Info": {"Description"},
		"License Info":     licenseAssembleOrder,
		"Summary Info":     {"Summary1", "Summary2"},
	}
	var b strings.Builder
	for idx, section := range orderedSections {
		b.WriteString("[")
		b.WriteString(section)
		b.WriteString("]\n")
		for _, key := range sectionOrder[section] {
			if value, ok := sections[section][key]; ok {
				b.WriteString(key)
				b.WriteString("=")
				b.WriteString(value)
				b.WriteString("\n")
			}
		}
		if idx < len(orderedSections)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func probeCPUId() string {
	switch runtime.GOOS {
	case "windows":
		return hardwareFirstNonEmpty(
			execValue("wmic", "cpu", "get", "ProcessorId", "/value"),
			execShellValue("powershell", "-NoProfile", "-Command", "(Get-CimInstance Win32_Processor | Select-Object -First 1 -ExpandProperty ProcessorId)"),
		)
	case "linux":
		return hardwareFirstNonEmpty(
			readKVFile("/proc/cpuinfo", "Serial"),
			readKVFile("/proc/cpuinfo", "model name"),
			readFileTrim("/etc/machine-id"),
		)
	case "darwin":
		return hardwareFirstNonEmpty(
			execShellValue("sysctl", "-n", "machdep.cpu.brand_string"),
			execShellValue("ioreg", "-rd1", "-c", "IOPlatformExpertDevice"),
		)
	default:
		return ""
	}
}

func probeBoardSerial() string {
	switch runtime.GOOS {
	case "windows":
		return hardwareFirstNonEmpty(
			execValue("wmic", "baseboard", "get", "serialnumber", "/value"),
			execShellValue("powershell", "-NoProfile", "-Command", "(Get-CimInstance Win32_BaseBoard | Select-Object -First 1 -ExpandProperty SerialNumber)"),
		)
	case "linux":
		return hardwareFirstNonEmpty(
			readFileTrim("/sys/class/dmi/id/board_serial"),
			readFileTrim("/sys/class/dmi/id/product_uuid"),
		)
	case "darwin":
		raw := execShellValue("ioreg", "-rd1", "-c", "IOPlatformExpertDevice")
		if raw != "" {
			for _, line := range strings.Split(raw, "\n") {
				if strings.Contains(line, "IOPlatformSerialNumber") {
					parts := strings.Split(line, "=")
					if len(parts) > 1 {
						return strings.Trim(strings.TrimSpace(parts[len(parts)-1]), "\"")
					}
				}
			}
		}
	}
	return ""
}

func probePrimaryMAC() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	candidates := make([]string, 0, len(ifaces))
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		mac := strings.TrimSpace(iface.HardwareAddr.String())
		if mac == "" || mac == "00:00:00:00:00:00" {
			continue
		}
		candidates = append(candidates, strings.ToUpper(mac))
	}
	sort.Strings(candidates)
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0]
}

func execValue(name string, args ...string) string {
	out := execShellValue(name, args...)
	if out == "" {
		return ""
	}
	if strings.Contains(out, "=") {
		for _, line := range strings.Split(out, "\n") {
			if idx := strings.Index(line, "="); idx >= 0 {
				value := strings.TrimSpace(line[idx+1:])
				if value != "" {
					return value
				}
			}
		}
	}
	lines := strings.Fields(out)
	if len(lines) > 1 {
		return strings.TrimSpace(lines[len(lines)-1])
	}
	return strings.TrimSpace(out)
}

func execShellValue(name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &bytes.Buffer{}
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(stdout.String())
}

func readKVFile(path, key string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	prefix := key + ":"
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func readFileTrim(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func parseLicenseINI(content string) (licenseSections, error) {
	sections := licenseSections{}
	current := ""
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			current = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			if _, ok := sections[current]; !ok {
				sections[current] = map[string]string{}
			}
			continue
		}
		if current == "" {
			continue
		}
		idx := strings.Index(line, "=")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		value = strings.Trim(value, "\"")
		sections[current][key] = value
	}
	if len(sections) == 0 {
		return nil, errors.New("invalid license file")
	}
	return sections, nil
}

func assembleLicenseValues(sections licenseSections) string {
	info := sections["License Info"]
	var b strings.Builder
	for _, key := range licenseAssembleOrder {
		b.WriteString(strings.TrimSpace(info[key]))
	}
	return b.String()
}

func verifyLicenseSummary(content string, localMachineCode string) (LicenseInfoPayload, error) {
	sections, err := parseLicenseINI(content)
	if err != nil {
		return defaultLicenseInfo(), err
	}
	info := sections["License Info"]
	summary := sections["Summary Info"]
	if strings.TrimSpace(info["Machine Code"]) == "" {
		return defaultLicenseInfo(), errors.New("授权文件缺少 Machine Code")
	}
	if localMachineCode != "" && strings.TrimSpace(info["Machine Code"]) != strings.TrimSpace(localMachineCode) {
		return defaultLicenseInfo(), fmt.Errorf("机器码不匹配：授权=%s，本机=%s", info["Machine Code"], localMachineCode)
	}
	md5Value := fmt.Sprintf("%x", md5.Sum([]byte(assembleLicenseValues(sections))))
	if err := verifySummary1(summary["Summary1"], md5Value); err != nil {
		return defaultLicenseInfo(), err
	}
	expires := info["Expire Time"]
	status := "已授权"
	if expires != "" {
		if parsed, parseErr := time.Parse("2006-01-02", expires); parseErr == nil && time.Now().After(parsed.Add(24*time.Hour)) {
			status = "已过期"
		}
	}
	features := parseExtendFeatures(info["Extend Info"])
	return LicenseInfoPayload{
		Status:              status,
		ExpireAt:            hardwareFirstNonEmpty(expires, "-"),
		ActiveAt:            hardwareFirstNonEmpty(info["Active Time"], "-"),
		MaintenanceExpireAt: hardwareFirstNonEmpty(info["Maintenance Expire Time"], "-"),
		LicenseTime:         hardwareFirstNonEmpty(info["License Time"], "-"),
		ProductType:         hardwareFirstNonEmpty(info["Product Type"], "-"),
		ProductTypeName:     productTypeDisplayName(info["Product Type"]),
		LicenseType:         hardwareFirstNonEmpty(info["License Type"], "-"),
		LicenseCounter:      hardwareFirstNonEmpty(info["License Counter"], "-"),
		MachineCode:         hardwareFirstNonEmpty(info["Machine Code"], "-"),
		ProjectCode:         hardwareFirstNonEmpty(info["Project Code"], "-"),
		RegionInfo:          hardwareFirstNonEmpty(info["Region Info"], "-"),
		IndustryInfo:        hardwareFirstNonEmpty(info["Industry Info"], "-"),
		CustomerInfo:        hardwareFirstNonEmpty(info["Customer Info"], "-"),
		UserInfo:            hardwareFirstNonEmpty(info["User Info"], "-"),
		ServerInfo:          hardwareFirstNonEmpty(info["Server Info"], "-"),
		Summary1:            hardwareFirstNonEmpty(summary["Summary1"], "-"),
		Summary2:            hardwareFirstNonEmpty(summary["Summary2"], "-"),
		Features:            features,
		LastVerifyResult:    "校验通过",
		RawLicenseContent:   content,
	}, nil
}

func verifySummary1(summary1 string, expectedMD5 string) error {
	if strings.TrimSpace(summary1) == "" {
		return errors.New("授权文件缺少 Summary1")
	}
	cipherText, err := base64.StdEncoding.DecodeString(strings.TrimSpace(summary1))
	if err != nil {
		return fmt.Errorf("Summary1 不是有效的 base64：%w", err)
	}
	privateKey, err := currentLicensePrivateKey()
	if err != nil {
		return err
	}
	decrypted, err := rsa.DecryptPKCS1v15(rand.Reader, privateKey, cipherText)
	if err != nil {
		return fmt.Errorf("Summary1 RSA 解密失败：%w", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(decrypted)))
	if err != nil {
		return fmt.Errorf("Summary1 明文 base64 解码失败：%w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(string(decoded)), expectedMD5) {
		return errors.New("授权校验失败：Summary1 与文件内容不一致")
	}
	return nil
}

func currentLicensePrivateKey() (*rsa.PrivateKey, error) {
	pemText := strings.TrimSpace(os.Getenv("SIPTUNNEL_LICENSE_RSA_PRIVATE_PEM"))
	if pemText == "" {
		pemText = defaultLicensePrivateKeyPEM
	}
	block, _ := pem.Decode([]byte(pemText))
	if block == nil {
		return nil, errors.New("无法解析授权私钥 PEM")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	return nil, errors.New("无法加载 RSA 私钥")
}

func parseExtendFeatures(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return []string{}
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return []string{raw}
	}
	out := make([]string, 0, len(obj))
	keys := make([]string, 0, len(obj))
	for key := range obj {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := obj[key]
		switch v := value.(type) {
		case bool:
			if v {
				out = append(out, key)
			}
		case string:
			if strings.TrimSpace(v) != "" {
				out = append(out, fmt.Sprintf("%s=%s", key, v))
			}
		case float64:
			out = append(out, fmt.Sprintf("%s=%s", key, strconv.FormatFloat(v, 'f', -1, 64)))
		default:
			out = append(out, key)
		}
	}
	return out
}
