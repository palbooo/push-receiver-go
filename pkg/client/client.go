package client

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/palbooo/push-receiver-go/internal/constants"
	"github.com/palbooo/push-receiver-go/internal/gcm"
	"github.com/palbooo/push-receiver-go/internal/parser"
	pb "github.com/palbooo/push-receiver-go/proto"
	"google.golang.org/protobuf/proto"
)

// EventType represents the type of event received from FCM
type EventType string

const (
	// EventConnect is emitted when the client successfully connects to FCM
	EventConnect EventType = "CONNECT"
	// EventDisconnect is emitted when the client disconnects from FCM
	EventDisconnect EventType = "DISCONNECT"
	// EventDataReceived is emitted when a data message is received
	EventDataReceived EventType = "ON_DATA_RECEIVED"
	// EventNotificationReceived is emitted when a notification is received
	EventNotificationReceived EventType = "ON_NOTIFICATION_RECEIVED"
	// EventError is emitted when an error occurs
	EventError EventType = "ERROR"
)

// Event represents an event from the FCM client
type Event struct {
	Type EventType
	Data interface{}
}

// Client represents an FCM push receiver client
type Client struct {
	androidID       string
	securityToken   string
	persistentIDs   []string
	conn            *tls.Conn
	parser          *parser.Parser
	eventChan       chan Event
	retryCount      int
	maxRetryTimeout int
	mu              sync.RWMutex
	closed          bool
	closeChan       chan struct{}
}

// NewClient creates a new FCM push receiver client
func NewClient(androidID, securityToken string, persistentIDs []string) *Client {
	if persistentIDs == nil {
		persistentIDs = []string{}
	}

	return &Client{
		androidID:       androidID,
		securityToken:   securityToken,
		persistentIDs:   persistentIDs,
		eventChan:       make(chan Event, 100),
		maxRetryTimeout: 15,
		closeChan:       make(chan struct{}),
	}
}

// Events returns a channel that receives events from the client
func (c *Client) Events() <-chan Event {
	return c.eventChan
}

// Connect establishes a connection to FCM and starts listening for messages
func (c *Client) Connect() error {
	// Perform GCM check-in
	if _, err := gcm.CheckIn(c.androidID, c.securityToken); err != nil {
		return fmt.Errorf("check-in failed: %w", err)
	}

	// Connect to MCS server
	if err := c.connect(); err != nil {
		return err
	}

	// Start listening for messages
	go c.listen()

	return nil
}

// Close closes the connection to FCM
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true
	close(c.closeChan)

	if c.conn != nil {
		return c.conn.Close()
	}

	return nil
}

// connect establishes a TLS connection and sends the login request
func (c *Client) connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Create TLS connection
	config := &tls.Config{
		ServerName: constants.MCSHost,
	}

	addr := constants.MCSHost + ":" + constants.MCSPort
	conn, err := tls.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("failed to connect to MCS: %w", err)
	}

	c.conn = conn
	c.parser = parser.NewParser(conn)

	// Send login request
	loginBuf, err := c.buildLoginRequest()
	if err != nil {
		c.conn.Close()
		return fmt.Errorf("failed to build login request: %w", err)
	}

	if _, err := c.conn.Write(loginBuf); err != nil {
		c.conn.Close()
		return fmt.Errorf("failed to send login request: %w", err)
	}

	return nil
}

// buildLoginRequest creates the login request buffer
func (c *Client) buildLoginRequest() ([]byte, error) {
	// Convert androidID to hex
	androidIDInt, err := strconv.ParseUint(c.androidID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid android ID: %w", err)
	}
	hexAndroidID := strconv.FormatUint(androidIDInt, 16)

	// Build login request
	authService := pb.LoginRequest_ANDROID_ID
	loginReq := &pb.LoginRequest{
		AdaptiveHeartbeat: proto.Bool(false),
		AuthService:       &authService,
		AuthToken:         proto.String(c.securityToken),
		Id:                proto.String("chrome-63.0.3234.0"),
		Domain:            proto.String("mcs.android.com"),
		DeviceId:          proto.String(fmt.Sprintf("android-%s", hexAndroidID)),
		NetworkType:       proto.Int32(1),
		Resource:          proto.String(c.androidID),
		User:              proto.String(c.androidID),
		UseRmq2:           proto.Bool(true),
		Setting: []*pb.Setting{
			{
				Name:  proto.String("new_vc"),
				Value: proto.String("1"),
			},
		},
		ClientEvent:          []*pb.ClientEvent{},
		ReceivedPersistentId: c.persistentIDs,
	}

	// Marshal the protobuf
	data, err := proto.Marshal(loginReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal login request: %w", err)
	}

	// Build the complete message with version, tag, and size
	var buf bytes.Buffer
	buf.WriteByte(constants.MCSVersion)
	buf.WriteByte(constants.LoginRequestTag)
	buf.Write(parser.EncodeVarint(uint32(len(data))))
	buf.Write(data)

	return buf.Bytes(), nil
}

// listen continuously reads messages from the connection
func (c *Client) listen() {
	defer func() {
		c.sendEvent(Event{Type: EventDisconnect})
		c.retry()
	}()

	for {
		select {
		case <-c.closeChan:
			return
		default:
		}

		msg, err := c.parser.ReadMessage()
		if err != nil {
			c.sendEvent(Event{Type: EventError, Data: err})
			return
		}

		c.handleMessage(msg)
	}
}

// handleMessage processes a received message
func (c *Client) handleMessage(msg *parser.Message) {
	switch msg.Tag {
	case constants.LoginResponseTag:
		c.mu.Lock()
		c.persistentIDs = []string{}
		c.retryCount = 0
		c.mu.Unlock()
		c.sendEvent(Event{Type: EventConnect})

	case constants.DataMessageStanzaTag:
		if dataMsg, ok := msg.Object.(*pb.DataMessageStanza); ok {
			c.handleDataMessage(dataMsg)
		}

	case constants.HeartbeatPingTag:
		// Respond to heartbeat ping with heartbeat ack
		c.sendHeartbeatAck()
	}
}

// handleDataMessage processes a data message stanza
func (c *Client) handleDataMessage(msg *pb.DataMessageStanza) {
	persistentID := msg.GetPersistentId()

	// Check if we've already received this message
	c.mu.RLock()
	for _, id := range c.persistentIDs {
		if id == persistentID {
			c.mu.RUnlock()
			return
		}
	}
	c.mu.RUnlock()

	// Add to persistent IDs
	c.mu.Lock()
	c.persistentIDs = append(c.persistentIDs, persistentID)
	c.mu.Unlock()

	// Convert app data to map
	appData := make(map[string]string)
	for _, data := range msg.AppData {
		appData[data.GetKey()] = data.GetValue()
	}

	// Check if message contains crypto-key (encrypted notification)
	if _, hasCryptoKey := appData["crypto-key"]; hasCryptoKey {
		// This would be a notification (encrypted)
		// For now, we'll emit it as a notification event
		// Full decryption would require additional crypto implementation
		c.sendEvent(Event{
			Type: EventNotificationReceived,
			Data: map[string]interface{}{
				"persistentId": persistentID,
				"from":         msg.GetFrom(),
				"category":     msg.GetCategory(),
				"appData":      appData,
				"rawData":      msg.GetRawData(),
			},
		})
	} else {
		// Unencrypted data message
		c.sendEvent(Event{
			Type: EventDataReceived,
			Data: map[string]interface{}{
				"persistentId": persistentID,
				"from":         msg.GetFrom(),
				"category":     msg.GetCategory(),
				"appData":      appData,
				"rawData":      msg.GetRawData(),
			},
		})
	}
}

// sendHeartbeatAck sends a heartbeat acknowledgment to the server
func (c *Client) sendHeartbeatAck() {
	ack := &pb.HeartbeatAck{}
	data, err := proto.Marshal(ack)
	if err != nil {
		return
	}

	var buf bytes.Buffer
	buf.WriteByte(constants.HeartbeatAckTag)
	buf.Write(parser.EncodeVarint(uint32(len(data))))
	buf.Write(data)

	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn != nil {
		conn.Write(buf.Bytes())
	}
}

// sendEvent sends an event to the event channel
func (c *Client) sendEvent(event Event) {
	select {
	case c.eventChan <- event:
	case <-time.After(time.Second):
		// Drop event if channel is full
	}
}

// retry attempts to reconnect to FCM with exponential backoff
func (c *Client) retry() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	c.retryCount++
	timeout := c.retryCount
	if timeout > c.maxRetryTimeout {
		timeout = c.maxRetryTimeout
	}

	time.Sleep(time.Duration(timeout) * time.Second)

	// Attempt to reconnect
	if err := c.connect(); err == nil {
		go c.listen()
	} else {
		go c.retry()
	}
}

