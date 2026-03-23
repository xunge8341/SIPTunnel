package server

import (
	"sort"
	"strings"
	"sync"

	"siptunnel/internal/config"
	"siptunnel/internal/protocol/manscdp"
	"siptunnel/internal/tunnelmapping"
)

type VirtualResource struct {
	DeviceID              string   `json:"device_id"`
	Name                  string   `json:"name"`
	Status                string   `json:"status"`
	MethodList            []string `json:"method_list"`
	ResponseMode          string   `json:"response_mode"`
	MaxInlineResponseBody int64    `json:"max_inline_response_body"`
	MaxRequestBody        int64    `json:"max_request_body"`
}

type CatalogRegistry struct {
	mu              sync.RWMutex
	resources       map[string]VirtualResource
	remoteResources map[string]VirtualResource
	portToDevice    map[int]string
	deviceToPorts   map[string][]int
}

func NewCatalogRegistry() *CatalogRegistry {
	return &CatalogRegistry{
		resources:       map[string]VirtualResource{},
		remoteResources: map[string]VirtualResource{},
		portToDevice:    map[int]string{},
		deviceToPorts:   map[string][]int{},
	}
}

func (r *CatalogRegistry) SyncLocalResources(items []LocalResourceRecord, mode config.NetworkMode) {
	r.mu.Lock()
	defer r.mu.Unlock()
	resources := make(map[string]VirtualResource, len(items))
	for _, item := range items {
		vr := localResourceToVirtualResource(item, mode)
		resources[vr.DeviceID] = vr
	}
	r.resources = resources
	if r.remoteResources == nil {
		r.remoteResources = map[string]VirtualResource{}
	}
	if r.portToDevice == nil {
		r.portToDevice = map[int]string{}
	}
	if r.deviceToPorts == nil {
		r.deviceToPorts = map[string][]int{}
	}
}

func (r *CatalogRegistry) SyncMappings(items []tunnelmapping.TunnelMapping) {
	r.mu.Lock()
	defer r.mu.Unlock()
	resources, portToDevice, deviceToPorts := virtualResourcesFromMappings(items)
	r.resources = resources
	if r.remoteResources == nil {
		r.remoteResources = map[string]VirtualResource{}
	}
	r.portToDevice = portToDevice
	r.deviceToPorts = deviceToPorts
}

func (r *CatalogRegistry) SyncExposureMappings(items []tunnelmapping.TunnelMapping) {
	r.mu.Lock()
	defer r.mu.Unlock()
	portToDevice, deviceToPorts := exposureBindingsFromMappings(items)
	r.portToDevice = portToDevice
	r.deviceToPorts = deviceToPorts
}

func (r *CatalogRegistry) SyncRemoteCatalog(items []manscdp.CatalogDevice, exposure []tunnelmapping.TunnelMapping) {
	r.mu.Lock()
	defer r.mu.Unlock()

	previousPorts := cloneDevicePortMap(r.deviceToPorts)
	resources := make(map[string]VirtualResource, len(items))
	for _, item := range items {
		deviceID := strings.TrimSpace(item.DeviceID)
		if deviceID == "" {
			continue
		}
		resources[deviceID] = VirtualResource{
			DeviceID:              deviceID,
			Name:                  firstNonEmpty(strings.TrimSpace(item.Name), deviceID),
			Status:                firstNonEmpty(strings.TrimSpace(item.Status), "ON"),
			MethodList:            allowedMethods(splitMethods(item.MethodList)),
			ResponseMode:          normalizedResponseMode(item.ResponseMode),
			MaxInlineResponseBody: item.MaxInlineResponseBody,
			MaxRequestBody:        item.MaxRequestBody,
		}
	}

	portToDevice, deviceToPorts := exposureBindingsFromMappings(exposure)
	for deviceID, ports := range previousPorts {
		if len(deviceToPorts[deviceID]) > 0 {
			continue
		}
		for _, port := range ports {
			if _, exists := portToDevice[port]; exists {
				continue
			}
			portToDevice[port] = deviceID
			deviceToPorts[deviceID] = append(deviceToPorts[deviceID], port)
		}
		if len(deviceToPorts[deviceID]) > 1 {
			sort.Ints(deviceToPorts[deviceID])
		}
	}
	for deviceID, ports := range deviceToPorts {
		if _, ok := resources[deviceID]; ok {
			continue
		}
		resources[deviceID] = VirtualResource{
			DeviceID:   deviceID,
			Name:       deviceID,
			Status:     "UNKNOWN",
			MethodList: []string{"*"},
		}
		deviceToPorts[deviceID] = append([]int(nil), ports...)
	}

	r.resources = resources
	r.remoteResources = cloneResources(resources)
	r.portToDevice = portToDevice
	r.deviceToPorts = deviceToPorts
}

func (r *CatalogRegistry) ResolveDeviceID(localPort int) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.portToDevice[localPort]
	return v, ok
}

func (r *CatalogRegistry) Resource(deviceID string) (VirtualResource, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.resources[strings.TrimSpace(deviceID)]
	return v, ok
}

func (r *CatalogRegistry) Snapshot() []VirtualResource {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]VirtualResource, 0, len(r.resources))
	for _, v := range r.resources {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DeviceID < out[j].DeviceID })
	return out
}

func (r *CatalogRegistry) RemoteSnapshot() []VirtualResource {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]VirtualResource, 0, len(r.remoteResources))
	for _, v := range r.remoteResources {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DeviceID < out[j].DeviceID })
	return out
}

func (r *CatalogRegistry) PortsForDevice(deviceID string) []int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ports := r.deviceToPorts[strings.TrimSpace(deviceID)]
	return append([]int(nil), ports...)
}

func (r *CatalogRegistry) ExposureSnapshot() map[string][]int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return cloneDevicePortMap(r.deviceToPorts)
}

func virtualResourcesFromMappings(items []tunnelmapping.TunnelMapping) (map[string]VirtualResource, map[int]string, map[string][]int) {
	resources := make(map[string]VirtualResource, len(items))
	portToDevice, deviceToPorts := exposureBindingsFromMappings(items)
	for _, item := range items {
		deviceID := item.EffectiveDeviceID()
		resources[deviceID] = VirtualResource{
			DeviceID:              deviceID,
			Name:                  firstNonEmpty(strings.TrimSpace(item.Name), item.MappingID),
			Status:                boolStatus(item.Enabled),
			MethodList:            allowedMethods(item.AllowedMethods),
			ResponseMode:          normalizedResponseMode(item.ResponseMode),
			MaxInlineResponseBody: item.MaxInlineResponseBody,
			MaxRequestBody:        item.MaxRequestBodyBytes,
		}
	}
	return resources, portToDevice, deviceToPorts
}

func exposureBindingsFromMappings(items []tunnelmapping.TunnelMapping) (map[int]string, map[string][]int) {
	portToDevice := make(map[int]string, len(items))
	deviceToPorts := make(map[string][]int, len(items))
	for _, item := range items {
		deviceID := item.EffectiveDeviceID()
		portToDevice[item.LocalBindPort] = deviceID
		deviceToPorts[deviceID] = append(deviceToPorts[deviceID], item.LocalBindPort)
	}
	for k := range deviceToPorts {
		sort.Ints(deviceToPorts[k])
	}
	return portToDevice, deviceToPorts
}

func cloneDevicePortMap(in map[string][]int) map[string][]int {
	out := make(map[string][]int, len(in))
	for deviceID, ports := range in {
		out[deviceID] = append([]int(nil), ports...)
	}
	return out
}

func cloneResources(in map[string]VirtualResource) map[string]VirtualResource {
	out := make(map[string]VirtualResource, len(in))
	for deviceID, item := range in {
		item.MethodList = append([]string(nil), item.MethodList...)
		out[deviceID] = item
	}
	return out
}

func splitMethods(v string) []string {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return nil
	}
	parts := strings.FieldsFunc(trimmed, func(r rune) bool { return r == ',' || r == ';' || r == '|' || r == ' ' || r == '\t' })
	out := make([]string, 0, len(parts))
	for _, item := range parts {
		if s := strings.TrimSpace(item); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func allowedMethods(methods []string) []string {
	if len(methods) == 0 {
		return []string{"*"}
	}
	out := make([]string, 0, len(methods))
	for _, item := range methods {
		trimmed := strings.ToUpper(strings.TrimSpace(item))
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return []string{"*"}
	}
	return out
}

func normalizedResponseMode(mode string) string {
	return tunnelmapping.NormalizeResponseMode(mode)
}

func boolStatus(enabled bool) string {
	if enabled {
		return "ON"
	}
	return "OFF"
}
