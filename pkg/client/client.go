package client

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"log"
	"net"
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
	// EventHeartbeatPing is emitted when a heartbeat ping is received from the server
	EventHeartbeatPing EventType = "HEARTBEAT_PING"
	// EventHeartbeatAck is emitted when we send a heartbeat ack
	EventHeartbeatAck EventType = "HEARTBEAT_ACK"
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
	debugMode       bool
	readTimeout     time.Duration
}

// ClientOption is a function that configures the client
type ClientOption func(*Client)

// WithDebugMode enables debug logging
func WithDebugMode(enabled bool) ClientOption {
	return func(c *Client) {
		c.debugMode = enabled
	}
}

// WithReadTimeout sets the read timeout for the connection
func WithReadTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) {
		c.readTimeout = timeout
	}
}

// NewClient creates a new FCM push receiver client
func NewClient(androidID, securityToken string, persistentIDs []string, opts ...ClientOption) *Client {
	if persistentIDs == nil {
		persistentIDs = []string{}
	}

	c := &Client{
		androidID:       androidID,
		securityToken:   securityToken,
		persistentIDs:   persistentIDs,
		eventChan:       make(chan Event, 100),
		maxRetryTimeout: 15,
		closeChan:       make(chan struct{}),
		debugMode:       false,
		readTimeout:     5 * time.Minute, // Default: 5 minutes (FCM sends heartbeat every ~4 min)
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Events returns a channel that receives events from the client
func (c *Client) Events() <-chan Event {
	return c.eventChan
}

// Connect establishes a connection to FCM and starts listening for messages
func (c *Client) Connect() error {
	c.debugLog("Starting connection to FCM...")

	// Perform GCM check-in
	c.debugLog("Performing GCM check-in...")
	if _, err := gcm.CheckIn(c.androidID, c.securityToken); err != nil {
		return fmt.Errorf("check-in failed: %w", err)
	}
	c.debugLog("GCM check-in successful")

	// Connect to MCS server
	if err := c.connect(); err != nil {
		return err
	}

	// Start listening for messages
	go c.listen()

	// Start sending heartbeat pings to keep connection alive
	go c.heartbeatLoop()

	return nil
}

// Close closes the connection to FCM
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.debugLog("Closing FCM connection...")
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

	// Create TLS connection with timeout
	config := &tls.Config{
		ServerName: constants.MCSHost,
	}

	addr := constants.MCSHost + ":" + constants.MCSPort
	c.debugLog("Connecting to %s...", addr)

	// Use dialer with timeout and keep-alive
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second, // TCP keep-alive to prevent NAT/firewall timeouts
	}

	netConn, err := tls.DialWithDialer(dialer, "tcp", addr, config)
	if err != nil {
		return fmt.Errorf("failed to connect to MCS: %w", err)
	}

	c.conn = netConn
	c.parser = parser.NewParser(netConn)
	c.debugLog("TLS connection established")

	// Send login request
	loginBuf, err := c.buildLoginRequest()
	if err != nil {
		c.conn.Close()
		return fmt.Errorf("failed to build login request: %w", err)
	}

	c.debugLog("Sending login request...")
	if _, err := c.conn.Write(loginBuf); err != nil {
		c.conn.Close()
		return fmt.Errorf("failed to send login request: %w", err)
	}
	c.debugLog("Login request sent")

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
		c.debugLog("Listen loop exiting, sending disconnect event...")
		c.sendEvent(Event{Type: EventDisconnect})
		c.retry()
	}()

	messageCount := 0
	for {
		select {
		case <-c.closeChan:
			c.debugLog("Close signal received, exiting listen loop")
			return
		default:
		}

		// Set read deadline to detect dead connections
		c.mu.RLock()
		conn := c.conn
		c.mu.RUnlock()

		if conn != nil {
			conn.SetReadDeadline(time.Now().Add(c.readTimeout))
		}

		msg, err := c.parser.ReadMessage()
		if err != nil {
			// Check if it's a timeout error
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				c.debugLog("Read timeout after %v - connection may be dead", c.readTimeout)
				c.sendEvent(Event{Type: EventError, Data: fmt.Errorf("read timeout: %w", err)})
			} else {
				c.debugLog("Read error: %v", err)
				c.sendEvent(Event{Type: EventError, Data: err})
			}
			return
		}

		messageCount++
		c.debugLog("Received message #%d, tag: %d", messageCount, msg.Tag)
		c.handleMessage(msg)
	}
}

// handleMessage processes a received message
func (c *Client) handleMessage(msg *parser.Message) {
	switch msg.Tag {
	case constants.LoginResponseTag:
		c.debugLog("Received LoginResponse - connection authenticated")
		c.mu.Lock()
		c.persistentIDs = []string{}
		c.retryCount = 0
		c.mu.Unlock()
		c.sendEvent(Event{Type: EventConnect})

	case constants.DataMessageStanzaTag:
		c.debugLog("Received DataMessageStanza")
		if dataMsg, ok := msg.Object.(*pb.DataMessageStanza); ok {
			c.handleDataMessage(dataMsg)
		}

	case constants.HeartbeatPingTag:
		c.debugLog("Received HeartbeatPing from server")
		c.sendEvent(Event{Type: EventHeartbeatPing, Data: time.Now()})
		c.sendHeartbeatAck()

	case constants.HeartbeatAckTag:
		c.debugLog("Received HeartbeatAck from server (unexpected)")

	case constants.CloseTag:
		c.debugLog("Received Close message from server")
		c.sendEvent(Event{Type: EventError, Data: fmt.Errorf("server sent close message")})

	case constants.IqStanzaTag:
		c.debugLog("Received IqStanza (ignoring)")

	default:
		c.debugLog("Received unknown message tag: %d", msg.Tag)
	}
}

// handleDataMessage processes a data message stanza
func (c *Client) handleDataMessage(msg *pb.DataMessageStanza) {
	persistentID := msg.GetPersistentId()
	c.debugLog("Processing DataMessage with persistentId: %s", persistentID)

	// Check if we've already received this message
	c.mu.RLock()
	for _, id := range c.persistentIDs {
		if id == persistentID {
			c.mu.RUnlock()
			c.debugLog("Duplicate message, ignoring (persistentId: %s)", persistentID)
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
		c.debugLog("Emitting EventNotificationReceived (encrypted)")
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
		c.debugLog("Emitting EventDataReceived")
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
		c.debugLog("Failed to marshal HeartbeatAck: %v", err)
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
		n, err := conn.Write(buf.Bytes())
		if err != nil {
			c.debugLog("Failed to send HeartbeatAck: %v", err)
			c.sendEvent(Event{Type: EventError, Data: fmt.Errorf("failed to send heartbeat ack: %w", err)})
		} else {
			c.debugLog("Sent HeartbeatAck (%d bytes)", n)
			c.sendEvent(Event{Type: EventHeartbeatAck, Data: time.Now()})
		}
	} else {
		c.debugLog("Cannot send HeartbeatAck: connection is nil")
	}
}

// heartbeatLoop sends periodic heartbeat pings to keep the connection alive
func (c *Client) heartbeatLoop() {
	// Send heartbeat every 4 minutes (FCM expects heartbeat within ~5 min)
	ticker := time.NewTicker(4 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-c.closeChan:
			c.debugLog("Heartbeat loop exiting (client closed)")
			return
		case <-ticker.C:
			c.sendHeartbeatPing()
		}
	}
}

// sendHeartbeatPing sends a heartbeat ping to keep the connection alive
func (c *Client) sendHeartbeatPing() {
	c.mu.RLock()
	if c.closed || c.conn == nil {
		c.mu.RUnlock()
		return
	}
	conn := c.conn
	c.mu.RUnlock()

	ping := &pb.HeartbeatPing{}
	data, err := proto.Marshal(ping)
	if err != nil {
		c.debugLog("Failed to marshal HeartbeatPing: %v", err)
		return
	}

	var buf bytes.Buffer
	buf.WriteByte(constants.HeartbeatPingTag)
	buf.Write(parser.EncodeVarint(uint32(len(data))))
	buf.Write(data)

	n, err := conn.Write(buf.Bytes())
	if err != nil {
		c.debugLog("Failed to send HeartbeatPing: %v", err)
	} else {
		c.debugLog("Sent HeartbeatPing (%d bytes)", n)
	}
}

// sendEvent sends an event to the event channel
func (c *Client) sendEvent(event Event) {
	select {
	case c.eventChan <- event:
	case <-time.After(time.Second):
		c.debugLog("Warning: Event channel full, dropping event: %s", event.Type)
	}
}

// retry attempts to reconnect to FCM with exponential backoff
func (c *Client) retry() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		c.debugLog("Client is closed, not retrying")
		return
	}

	c.retryCount++
	timeout := c.retryCount
	if timeout > c.maxRetryTimeout {
		timeout = c.maxRetryTimeout
	}

	c.debugLog("Retrying connection in %d seconds (attempt %d)...", timeout, c.retryCount)
	time.Sleep(time.Duration(timeout) * time.Second)

	// Attempt to reconnect
	if err := c.connect(); err == nil {
		c.debugLog("Reconnection successful")
		go c.listen()
	} else {
		c.debugLog("Reconnection failed: %v", err)
		go c.retry()
	}
}

// debugLog logs a message if debug mode is enabled
func (c *Client) debugLog(format string, args ...interface{}) {
	if c.debugMode {
		log.Printf("[FCM_CLIENT] "+format, args...)
	}
}
