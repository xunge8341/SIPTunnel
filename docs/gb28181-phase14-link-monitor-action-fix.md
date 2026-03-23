# Phase 14: Link monitor action response typing fix

Fix UI typecheck failure in `LinkMonitorView.vue`.

## Problem

`gatewayApi.triggerTunnelSessionAction()` returns the unwrapped `data` payload only, whose
TypeScript type is `TunnelSessionActionResponse`.
That type does not define a `message` field, but `LinkMonitorView.vue` accessed `result.message`.

## Fix

The view now derives a local human-readable action label and no longer depends on a non-existent
`message` field in the response payload.

This keeps the UI aligned with the current backend response contract.
