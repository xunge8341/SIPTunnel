# Phase 12: duplicate firstNonEmpty build fix

This phase removes the duplicate top-level `firstNonEmpty` helper from
`gateway-server/internal/server/catalog_registry.go`.

The `server` package already defines the same helper in `http.go`, so keeping
both declarations causes Go to fail package compilation with:

```
firstNonEmpty redeclared in this block
```

After this change, `catalog_registry.go` reuses the shared helper defined in
`http.go`.
