package server

import (
	"sort"
	"strings"

	"siptunnel/internal/tunnelmapping"
)

type catalogExposureView struct {
	DeviceID              string   `json:"device_id"`
	Name                  string   `json:"name"`
	Status                string   `json:"status"`
	MethodList            []string `json:"method_list"`
	ResponseMode          string   `json:"response_mode"`
	MaxInlineResponseBody int64    `json:"max_inline_response_body"`
	MaxRequestBody        int64    `json:"max_request_body"`
	LocalPorts            []int    `json:"local_ports"`
	ExposureMode          string   `json:"exposure_mode"`
	MappingIDs            []string `json:"mapping_ids,omitempty"`
}

type catalogExposurePlan struct {
	EffectiveMappings []tunnelmapping.TunnelMapping
	Views             []catalogExposureView
}

// buildCatalogExposurePlan 只反映当前已经生效的“手工映射 vs 未映射”事实。
//
// 历史上这里还尝试过目录驱动的自动暴露/自动映射，但当前主线已经收口到：
// 1. 目录只提供资源发现；
// 2. 手工映射才是运行时唯一事实来源；
// 3. 控制台与日志统一展示 MANUAL / UNEXPOSED，不再制造 AUTO 幻象。
func buildCatalogExposurePlan(static []tunnelmapping.TunnelMapping, resources []VirtualResource) catalogExposurePlan {
	plan := catalogExposurePlan{}
	if len(static) > 0 {
		plan.EffectiveMappings = append(plan.EffectiveMappings, cloneMappings(static)...)
	}

	byDevice := make(map[string][]tunnelmapping.TunnelMapping, len(static))
	for _, item := range static {
		deviceID := strings.TrimSpace(item.EffectiveDeviceID())
		if deviceID != "" {
			byDevice[deviceID] = append(byDevice[deviceID], item)
		}
	}

	orderedResources := append([]VirtualResource(nil), resources...)
	sort.Slice(orderedResources, func(i, j int) bool { return orderedResources[i].DeviceID < orderedResources[j].DeviceID })

	for _, resource := range orderedResources {
		deviceID := strings.TrimSpace(resource.DeviceID)
		if deviceID == "" {
			continue
		}
		view := exposureViewFromResource(resource)
		if items := byDevice[deviceID]; len(items) > 0 {
			view.ExposureMode = "MANUAL"
			view.LocalPorts = collectPorts(items)
			view.MappingIDs = collectMappingIDs(items)
		} else {
			view.ExposureMode = "UNEXPOSED"
		}
		plan.Views = append(plan.Views, view)
	}
	return plan
}

func exposureViewFromResource(resource VirtualResource) catalogExposureView {
	return catalogExposureView{
		DeviceID:              resource.DeviceID,
		Name:                  resource.Name,
		Status:                resource.Status,
		MethodList:            append([]string(nil), resource.MethodList...),
		ResponseMode:          normalizedResponseMode(resource.ResponseMode),
		MaxInlineResponseBody: resource.MaxInlineResponseBody,
		MaxRequestBody:        resource.MaxRequestBody,
		LocalPorts:            []int{},
		ExposureMode:          "UNEXPOSED",
	}
}

func collectPorts(items []tunnelmapping.TunnelMapping) []int {
	out := make([]int, 0, len(items))
	for _, item := range items {
		out = append(out, item.LocalBindPort)
	}
	sort.Ints(out)
	return out
}

func collectMappingIDs(items []tunnelmapping.TunnelMapping) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.MappingID)
	}
	sort.Strings(out)
	return out
}

func cloneMappings(items []tunnelmapping.TunnelMapping) []tunnelmapping.TunnelMapping {
	out := make([]tunnelmapping.TunnelMapping, 0, len(items))
	for _, item := range items {
		cp := item
		cp.AllowedMethods = append([]string(nil), item.AllowedMethods...)
		out = append(out, cp)
	}
	return out
}
