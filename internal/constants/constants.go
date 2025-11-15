package constants

// MCS Protocol Version
const MCSVersion = 41

// Processing states
const (
	MCSVersionTagAndSize = iota
	MCSTagAndSize
	MCSSize
	MCSProtoBytes
)

// Packet lengths
const (
	VersionPacketLen = 1
	TagPacketLen     = 1
	SizePacketLenMin = 1
	SizePacketLenMax = 5
)

// MCS Message tags
const (
	HeartbeatPingTag       = 0
	HeartbeatAckTag        = 1
	LoginRequestTag        = 2
	LoginResponseTag       = 3
	CloseTag               = 4
	MessageStanzaTag       = 5
	PresenceStanzaTag      = 6
	IqStanzaTag            = 7
	DataMessageStanzaTag   = 8
	BatchPresenceStanzaTag = 9
	StreamErrorStanzaTag   = 10
	HttpRequestTag         = 11
	HttpResponseTag        = 12
	BindAccountRequestTag  = 13
	BindAccountResponseTag = 14
	TalkMetadataTag        = 15
	NumProtoTypes          = 16
)

// MCS Server configuration
const (
	MCSHost = "mtalk.google.com"
	MCSPort = "5228"
)

// GCM configuration
const (
	CheckinURL = "https://android.clients.google.com/checkin"
)

