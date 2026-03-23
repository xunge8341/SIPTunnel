package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"

	"siptunnel/internal/tunnelmapping"
)

func main() {
	in := flag.String("in", "", "legacy route file (json)")
	out := flag.String("out", "", "tunnel mapping file (json)")
	flag.Parse()

	if *in == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/mapping-migrate --in <legacy.json> --out <tunnel_mappings.json>")
		os.Exit(2)
	}

	buf, err := os.ReadFile(*in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read input failed: %v\n", err)
		os.Exit(1)
	}

	items, err := parseLegacyMappings(buf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "convert input failed: %v\n", err)
		os.Exit(1)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].MappingID < items[j].MappingID })

	payload, err := json.MarshalIndent(struct {
		Items []tunnelmapping.TunnelMapping `json:"items"`
	}{Items: items}, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal output failed: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, payload, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write output failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("migrated %d routes -> %s\n", len(items), *out)
}

func parseLegacyMappings(buf []byte) ([]tunnelmapping.TunnelMapping, error) {
	legacyOps := struct {
		Items  []tunnelmapping.LegacyOpsRoute `json:"items"`
		Routes []tunnelmapping.LegacyOpsRoute `json:"routes"`
	}{}
	if err := json.Unmarshal(buf, &legacyOps); err == nil {
		routes := legacyOps.Items
		if len(routes) == 0 {
			routes = legacyOps.Routes
		}
		if len(routes) > 0 {
			items := make([]tunnelmapping.TunnelMapping, 0, len(routes))
			for _, route := range routes {
				item, err := tunnelmapping.MappingFromLegacyOpsRoute(route)
				if err != nil {
					return nil, err
				}
				items = append(items, item)
			}
			return items, nil
		}
	}

	legacyRouteConfig := struct {
		Routes []tunnelmapping.LegacyRouteConfig `json:"routes"`
	}{}
	if err := json.Unmarshal(buf, &legacyRouteConfig); err == nil && len(legacyRouteConfig.Routes) > 0 {
		items := make([]tunnelmapping.TunnelMapping, 0, len(legacyRouteConfig.Routes))
		for _, route := range legacyRouteConfig.Routes {
			item, err := tunnelmapping.MappingFromLegacyRouteConfig(route)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		return items, nil
	}

	var routeArray []tunnelmapping.LegacyRouteConfig
	if err := json.Unmarshal(buf, &routeArray); err == nil && len(routeArray) > 0 {
		items := make([]tunnelmapping.TunnelMapping, 0, len(routeArray))
		for _, route := range routeArray {
			item, err := tunnelmapping.MappingFromLegacyRouteConfig(route)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		return items, nil
	}

	return nil, fmt.Errorf("no supported legacy route payload found")
}
