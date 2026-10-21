package dto

// GetOrCreateClientResult is the registration outcome handed to the transport
// adapter by the anonymous self-registration path (register_client,
// feat-whatsapp-bot D2): the resolved client id (byte-identical to the header
// phone), the stored display name and whether this call created the row.
//
// It lives in the DTO package because it is a cross-layer carrier, not a use-case
// detail: internal/mcp declares its RegisterClientPort in terms of this type, and
// ADR-0013 keeps DTOs as the bridge between layers — a transport contract must not
// be namespaced by a concrete use case (the deviation that feat-whatsapp-bot
// introduced and this carrier move closes). It stays wire-free by design: the
// transport maps it onto the pinned register_client output shape, so adding a
// field here never leaks to the LLM.
type GetOrCreateClientResult struct {
	ClientID    string
	DisplayName string
	Created     bool
}
