package manscdp

import (
	"encoding/xml"
	"strconv"
	"strings"
)

const ContentType = "Application/MANSCDP+xml"

type HeaderKV struct {
	Key   string `xml:"Key,attr,omitempty" json:"key,omitempty"`
	Value string `xml:",chardata" json:"value,omitempty"`
}

type CatalogDevice struct {
	DeviceID              string `xml:"DeviceID" json:"device_id"`
	Name                  string `xml:"Name,omitempty" json:"name,omitempty"`
	Status                string `xml:"Status,omitempty" json:"status,omitempty"`
	MethodList            string `xml:"MethodList,omitempty" json:"method_list,omitempty"`
	ResponseMode          string `xml:"ResponseMode,omitempty" json:"response_mode,omitempty"`
	MaxInlineResponseBody int64  `xml:"MaxInlineResponseBody,omitempty" json:"max_inline_response_body,omitempty"`
	MaxRequestBody        int64  `xml:"MaxRequestBody,omitempty" json:"max_request_body,omitempty"`
}

type CatalogNotify struct {
	XMLName    xml.Name        `xml:"Notify"`
	CmdType    string          `xml:"CmdType"`
	SN         int             `xml:"SN"`
	DeviceID   string          `xml:"DeviceID"`
	SumNum     int             `xml:"SumNum"`
	DeviceList []CatalogDevice `xml:"DeviceList>Item,omitempty"`
}

type CatalogQuery struct {
	XMLName  xml.Name `xml:"Query"`
	CmdType  string   `xml:"CmdType"`
	SN       int      `xml:"SN"`
	DeviceID string   `xml:"DeviceID"`
}

type KeepaliveNotify struct {
	XMLName  xml.Name `xml:"Notify"`
	CmdType  string   `xml:"CmdType"`
	SN       int      `xml:"SN"`
	DeviceID string   `xml:"DeviceID"`
	Status   string   `xml:"Status,omitempty"`
}

type DeviceControl struct {
	XMLName         xml.Name   `xml:"Control"`
	CmdType         string     `xml:"CmdType"`
	SN              int        `xml:"SN"`
	DeviceID        string     `xml:"DeviceID"`
	TunnelStage     string     `xml:"TunnelStage,omitempty"`
	Method          string     `xml:"Method,omitempty"`
	RequestPath     string     `xml:"RequestPath,omitempty"`
	RawQuery        string     `xml:"RawQuery,omitempty"`
	ResponseMode    string     `xml:"ResponseMode,omitempty"`
	StatusCode      int        `xml:"StatusCode,omitempty"`
	Reason          string     `xml:"Reason,omitempty"`
	ContentLength   int64      `xml:"ContentLength,omitempty"`
	Headers         []HeaderKV `xml:"Headers>Header,omitempty"`
	BodyBase64      string     `xml:"BodyBase64,omitempty"`
	BodyContentType string     `xml:"BodyContentType,omitempty"`
}

func Marshal(v any) ([]byte, error) {
	body, err := xml.Marshal(v)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func DetectCmdType(body []byte) string {
	var x struct {
		CmdType string `xml:"CmdType"`
	}
	if err := xml.Unmarshal(body, &x); err != nil {
		return ""
	}
	return strings.TrimSpace(x.CmdType)
}

func ParseCatalog(body []byte) (CatalogNotify, error) {
	var x CatalogNotify
	return x, xml.Unmarshal(body, &x)
}

func ParseKeepalive(body []byte) (KeepaliveNotify, error) {
	var x KeepaliveNotify
	return x, xml.Unmarshal(body, &x)
}

func ParseCatalogQuery(body []byte) (CatalogQuery, error) {
	var x CatalogQuery
	return x, xml.Unmarshal(body, &x)
}

func ParseDeviceControl(body []byte) (DeviceControl, error) {
	var x DeviceControl
	return x, xml.Unmarshal(body, &x)
}

func BuildCatalogQuery(deviceID string, sn int) []byte {
	body, err := Marshal(CatalogQuery{CmdType: "Catalog", SN: sn, DeviceID: strings.TrimSpace(deviceID)})
	if err != nil {
		return nil
	}
	return body
}

func normalizeDirection(direction string) string {
	switch strings.ToLower(strings.TrimSpace(direction)) {
	case "sendonly":
		return "sendonly"
	default:
		return "recvonly"
	}
}

func BuildRelaySDP(ip string, port int, subject string, deviceID string, direction string) []byte {
	direction = normalizeDirection(direction)
	lines := []string{
		"v=0",
		"o=" + sanitizeToken(deviceID, "device") + " 0 0 IN IP4 " + strings.TrimSpace(ip),
		"s=Play",
		"c=IN IP4 " + strings.TrimSpace(ip),
		"t=0 0",
		"m=video " + strconv.Itoa(port) + " RTP/AVP 96",
		"a=rtpmap:96 PS/90000",
		"a=" + direction,
	}
	lines = append(lines, "y="+buildSSRCString(deviceID))
	if strings.TrimSpace(deviceID) != "" {
		lines = append(lines, "a=deviceid:"+strings.TrimSpace(deviceID))
	}
	return []byte(strings.Join(lines, "\r\n") + "\r\n")
}

func buildSSRCString(deviceID string) string {
	digits := strings.Map(func(r rune) rune {
		if r < '0' || r > '9' {
			return -1
		}
		return r
	}, strings.TrimSpace(deviceID))
	if digits == "" {
		return "0000000001"
	}
	if len(digits) > 10 {
		digits = digits[len(digits)-10:]
	}
	if len(digits) < 10 {
		digits = strings.Repeat("0", 10-len(digits)) + digits
	}
	if digits == "0000000000" {
		return "0000000001"
	}
	return digits
}

func sanitizeToken(v, fallback string) string {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return fallback
	}
	trimmed = strings.ReplaceAll(trimmed, " ", "-")
	trimmed = strings.ReplaceAll(trimmed, ":", "-")
	return trimmed
}
