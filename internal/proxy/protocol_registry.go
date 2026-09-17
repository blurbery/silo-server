package proxy

import (
	"github.com/Silo-Server/silo-server/internal/playback"
	"github.com/Silo-Server/silo-server/internal/workerprotocol"
	"github.com/danielgtaylor/huma/v2"
)

// ProtocolReads describes the retained reads using the same DTOs their handlers
// encode. The bearer middleware and worker paths remain owned by this listener.
func ProtocolReads(schemas huma.Registry) []workerprotocol.Operation {
	return []workerprotocol.Operation{
		workerprotocol.JSONRead[playback.HWAccelInfo](schemas, "proxy", "/hw-capabilities", "(*internal/proxy.Server).handleHWCapabilities", 401, 503),
		workerprotocol.JSONRead[statusResponse](schemas, "proxy", "/status", "(*internal/proxy.Server).handleStatus", 401),
	}
}

// ProtocolControls describes the existing worker admin commands, not native API aliases.
func ProtocolControls(schemas huma.Registry) []workerprotocol.Operation {
	reprobe := workerprotocol.JSONRead[reprobeCapabilitiesResponse](schemas, "proxy", "/admin/reprobe-capabilities", "(*internal/proxy.Server).handleReprobeCapabilities", 401, 409, 503)
	reprobe.Method = "POST"
	reprobe.RetrySafety = "non_retryable"
	reprobe.Description = "Rebuild the capability snapshot. Busy probes refuse with 409; an incomplete probe retains the prior published hash. No durable replay receipt."
	return []workerprotocol.Operation{
		workerprotocol.EmptyCommand("proxy", "/admin/force-reload", "(*internal/proxy.Server).handleForceReload", "Reload configuration; active remux work is not torn down."),
		workerprotocol.EmptyCommand("proxy", "/admin/reload-config", "(*internal/proxy.Server).handleReloadConfig", "Reload configuration without tearing down active sessions. No durable replay receipt."),
		reprobe,
	}
}
