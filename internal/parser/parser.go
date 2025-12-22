package parser

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/palbooo/push-receiver-go/internal/constants"
	pb "github.com/palbooo/push-receiver-go/proto"
	"google.golang.org/protobuf/proto"
)

// Message represents a parsed MCS protocol message
type Message struct {
	Tag    uint8
	Object proto.Message
}

// Parser handles parsing of MCS protocol messages
type Parser struct {
	reader            io.Reader
	state             int
	data              []byte
	sizePacketSoFar   int
	messageTag        uint8
	messageSize       uint32
	handshakeComplete bool
}

// NewParser creates a new MCS protocol parser
func NewParser(reader io.Reader) *Parser {
	return &Parser{
		reader: reader,
		state:  constants.MCSVersionTagAndSize,
		data:   make([]byte, 0),
	}
}

// ReadMessage reads and parses the next message from the stream
func (p *Parser) ReadMessage() (*Message, error) {
	for {
		switch p.state {
		case constants.MCSVersionTagAndSize:
			if err := p.onGotVersion(); err != nil {
				return nil, err
			}

		case constants.MCSTagAndSize:
			if err := p.onGotMessageTag(); err != nil {
				return nil, err
			}

		case constants.MCSSize:
			if err := p.onGotMessageSize(); err != nil {
				return nil, err
			}

		case constants.MCSProtoBytes:
			return p.onGotMessageBytes()

		default:
			return nil, fmt.Errorf("unexpected parser state: %d", p.state)
		}
	}
}

// readBytes reads exactly n bytes from the reader
func (p *Parser) readBytes(n int) ([]byte, error) {
	buf := make([]byte, n)
	_, err := io.ReadFull(p.reader, buf)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

// readVarint reads a varint from the reader
func (p *Parser) readVarint() (uint32, int, error) {
	var value uint32
	var shift uint
	bytesRead := 0

	for bytesRead < constants.SizePacketLenMax {
		buf, err := p.readBytes(1)
		if err != nil {
			return 0, bytesRead, err
		}
		bytesRead++

		b := buf[0]
		value |= uint32(b&0x7F) << shift

		if b&0x80 == 0 {
			return value, bytesRead, nil
		}
		shift += 7
	}

	return 0, bytesRead, fmt.Errorf("varint too long")
}

// onGotVersion handles the version packet
func (p *Parser) onGotVersion() error {
	buf, err := p.readBytes(constants.VersionPacketLen)
	if err != nil {
		return fmt.Errorf("failed to read version: %w", err)
	}

	version := buf[0]
	if version != constants.MCSVersion && version != 38 {
		return fmt.Errorf("unexpected MCS version: %d", version)
	}

	// After version, we expect the login response tag
	p.state = constants.MCSTagAndSize
	return nil
}

// onGotMessageTag handles reading the message tag
func (p *Parser) onGotMessageTag() error {
	buf, err := p.readBytes(constants.TagPacketLen)
	if err != nil {
		return fmt.Errorf("failed to read message tag: %w", err)
	}

	p.messageTag = buf[0]
	p.state = constants.MCSSize
	return nil
}

// onGotMessageSize handles reading the message size
func (p *Parser) onGotMessageSize() error {
	size, _, err := p.readVarint()
	if err != nil {
		return fmt.Errorf("failed to read message size: %w", err)
	}

	p.messageSize = size

	if p.messageSize > 0 {
		p.state = constants.MCSProtoBytes
	} else {
		// Empty message, process immediately
		p.state = constants.MCSProtoBytes
	}

	return nil
}

// onGotMessageBytes handles reading and parsing the message body
func (p *Parser) onGotMessageBytes() (*Message, error) {
	var protoMsg proto.Message
	var err error

	// If message has content, read it
	if p.messageSize > 0 {
		buf, err := p.readBytes(int(p.messageSize))
		if err != nil {
			return nil, fmt.Errorf("failed to read message bytes: %w", err)
		}

		protoMsg, err = p.unmarshalByTag(p.messageTag, buf)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal message: %w", err)
		}
	} else {
		// Empty message - create default instance
		protoMsg, err = p.createEmptyMessage(p.messageTag)
		if err != nil {
			return nil, fmt.Errorf("failed to create empty message: %w", err)
		}
	}

	// Check if this is the login response (handshake complete)
	if p.messageTag == constants.LoginResponseTag {
		p.handshakeComplete = true
	}

	// Save the tag before resetting
	tag := p.messageTag

	// Prepare for next message
	p.messageTag = 0
	p.messageSize = 0
	p.state = constants.MCSTagAndSize

	return &Message{
		Tag:    tag,
		Object: protoMsg,
	}, nil
}

// unmarshalByTag unmarshals a protobuf message based on its tag
func (p *Parser) unmarshalByTag(tag uint8, data []byte) (proto.Message, error) {
	var msg proto.Message

	switch tag {
	case constants.HeartbeatPingTag:
		msg = &pb.HeartbeatPing{}
	case constants.HeartbeatAckTag:
		msg = &pb.HeartbeatAck{}
	case constants.LoginRequestTag:
		msg = &pb.LoginRequest{}
	case constants.LoginResponseTag:
		msg = &pb.LoginResponse{}
	case constants.CloseTag:
		msg = &pb.Close{}
	case constants.IqStanzaTag:
		msg = &pb.IqStanza{}
	case constants.DataMessageStanzaTag:
		msg = &pb.DataMessageStanza{}
	case constants.StreamErrorStanzaTag:
		msg = &pb.StreamErrorStanza{}
	default:
		return nil, fmt.Errorf("unknown message tag: %d", tag)
	}

	if err := proto.Unmarshal(data, msg); err != nil {
		return nil, err
	}

	return msg, nil
}

// createEmptyMessage creates an empty protobuf message for the given tag
func (p *Parser) createEmptyMessage(tag uint8) (proto.Message, error) {
	switch tag {
	case constants.HeartbeatPingTag:
		return &pb.HeartbeatPing{}, nil
	case constants.HeartbeatAckTag:
		return &pb.HeartbeatAck{}, nil
	case constants.LoginRequestTag:
		return &pb.LoginRequest{}, nil
	case constants.LoginResponseTag:
		return &pb.LoginResponse{}, nil
	case constants.CloseTag:
		return &pb.Close{}, nil
	case constants.IqStanzaTag:
		return &pb.IqStanza{}, nil
	case constants.DataMessageStanzaTag:
		return &pb.DataMessageStanza{}, nil
	case constants.StreamErrorStanzaTag:
		return &pb.StreamErrorStanza{}, nil
	default:
		return nil, fmt.Errorf("unknown message tag: %d", tag)
	}
}

// EncodeVarint encodes a uint32 as a varint
func EncodeVarint(value uint32) []byte {
	buf := make([]byte, binary.MaxVarintLen32)
	n := binary.PutUvarint(buf, uint64(value))
	return buf[:n]
}

