package gobridge

const (
	BridgeProtocolName           = "cccode-bridge"
	BridgeProtocolVersion        = 1
	BridgeProtocolSchemaRevision = "2026-05-07"
)

type BridgeV1Protocol struct {
	Name                     string   `json:"name"`
	Version                  int      `json:"version"`
	SchemaRevision           string   `json:"schemaRevision,omitempty"`
	SupportedSchemaRevisions []string `json:"supportedSchemaRevisions,omitempty"`
}

type BridgeV1Client struct {
	App      string `json:"app"`
	Version  string `json:"version"`
	DeviceID string `json:"deviceId"`
}

type BridgeV1Hello struct {
	Type     string           `json:"type"`
	Client   BridgeV1Client   `json:"client"`
	Protocol BridgeV1Protocol `json:"protocol"`
}

type BridgeV1CurrentURLs struct {
	Local   string   `json:"local"`
	Remote  *string  `json:"remote"`
	Remotes []string `json:"remotes,omitempty"`
}

type BridgeV1BridgeProfile struct {
	BridgeID       string                   `json:"bridgeId"`
	DisplayName    string                   `json:"displayName"`
	RuntimeVersion string                   `json:"runtimeVersion"`
	CurrentURLs    BridgeV1CurrentURLs      `json:"currentURLs"`
	Protocol       BridgeV1Protocol         `json:"protocol"`
	Security       *BridgeV1SecurityProfile `json:"security,omitempty"`
}

type BridgeV1SecurityProfile struct {
	Level            string `json:"level"`
	Scheme           string `json:"scheme,omitempty"`
	HostCategory     string `json:"hostCategory,omitempty"`
	IsTailscaleCGNAT bool   `json:"isTailscaleCGNAT,omitempty"`
	IsPublicWS       bool   `json:"isPublicWS,omitempty"`
}

type BridgeV1Capabilities struct {
	RemoteAccessConfig bool `json:"remoteAccessConfig"`
	TrustedDevices     bool `json:"trustedDevices"`
	OfflineSnapshots   bool `json:"offlineSnapshots"`
	WorkspaceList      bool `json:"workspaceList"`
	SessionMutation    bool `json:"sessionMutation"`
}

type BridgeV1RunningSession struct {
	BackendID   string `json:"backendId"`
	WorkspaceID string `json:"workspaceId,omitempty"`
	SessionID   string `json:"sessionId"`
	Status      string `json:"status"`
}

type BridgeV1HelloAck struct {
	Type            string                   `json:"type"`
	OK              bool                     `json:"ok"`
	Bridge          *BridgeV1BridgeProfile   `json:"bridge,omitempty"`
	Capabilities    *BridgeV1Capabilities    `json:"capabilities,omitempty"`
	Backends        []BackendInfo            `json:"backends,omitempty"`
	BridgeStatus    string                   `json:"bridgeStatus,omitempty"`
	RunningSessions []BridgeV1RunningSession `json:"runningSessions,omitempty"`
	Error           *WireError               `json:"error,omitempty"`
}

type BridgeV1PairingClaimParams struct {
	PairingID  string                `json:"pairingId,omitempty"`
	ManualCode string                `json:"manualCode,omitempty"`
	Device     BridgeV1PairingDevice `json:"device"`
}

type BridgeV1PairingDevice struct {
	DeviceID    string `json:"deviceId"`
	DisplayName string `json:"displayName"`
	Platform    string `json:"platform"`
}

type BridgeV1PairingResult struct {
	Type   string                    `json:"type"`
	OK     bool                      `json:"ok"`
	Device *BridgeV1AuthorizedDevice `json:"device,omitempty"`
	Bridge *BridgeV1PairedBridge     `json:"bridge,omitempty"`
	Error  *WireError                `json:"error,omitempty"`
}

type BridgeV1AuthorizedDevice struct {
	DeviceID string `json:"deviceId"`
	Token    string `json:"token"`
}

type BridgeV1PairedBridge struct {
	BridgeID    string   `json:"bridgeId"`
	DisplayName string   `json:"displayName"`
	LocalURL    string   `json:"localURL"`
	RemoteURL   *string  `json:"remoteURL"`
	RemoteURLs  []string `json:"remoteURLs,omitempty"`
}

type BridgeV1EventEnvelope struct {
	Type        string      `json:"type"`
	Seq         int         `json:"seq"`
	BackendID   string      `json:"backendId,omitempty"`
	WorkspaceID string      `json:"workspaceId,omitempty"`
	SessionID   string      `json:"sessionId,omitempty"`
	Event       string      `json:"event"`
	Data        interface{} `json:"data,omitempty"`
}
