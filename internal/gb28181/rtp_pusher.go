// Package gb28181 implements RTP packetization of PS streams and
// UDP push to the GB/T 28181 platform using github.com/pion/rtp.
package gb28181

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/pion/rtp"
)

// RtpPusher manages RTP packetization and UDP transmission of PS streams.
//
// SSRC is provided by the platform via INVITE SDP y= field (10-digit decimal),
// NOT derived from device_id+channel_id.
type RtpPusher struct {
	conn       *net.UDPConn
	remoteAddr *net.UDPAddr
	seqNum     uint16
	mu         sync.Mutex
}

// NewRtpPusher creates a new RTP pusher with the provided UDP connection.
// Initializes with a random sequence number.
func NewRtpPusher(conn *net.UDPConn, remoteAddr *net.UDPAddr) *RtpPusher {
	return &RtpPusher{
		conn:       conn,
		remoteAddr: remoteAddr,
		seqNum:     uint16(time.Now().UnixNano() & 0xFFFF), // Random init
	}
}

// BuildRtpPacket builds an RTP packet using pion/rtp.
// Returns the marshaled packet bytes ready for UDP transmission.
//
// payload: PS payload data
// marker: Marker bit (true only on last fragment of an access unit)
// ssrc: Platform-provided SSRC from INVITE SDP y= field
// seqNum: RTP sequence number
// timestamp: RTP timestamp in 90kHz ticks
func BuildRtpPacket(payload []byte, marker bool, ssrc uint32, seqNum uint16, timestamp uint32) ([]byte, error) {
	packet := &rtp.Packet{
		Header: rtp.Header{
			Version:        2, // RTP version 2
			PayloadType:    96, // PS over RTP per GB/T 28181
			Marker:         marker,
			SequenceNumber: seqNum,
			Timestamp:      timestamp,
			SSRC:           ssrc,
		},
		Payload: payload,
	}
	buf, err := packet.Marshal()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal RTP packet: %w", err)
	}
	return buf, nil
}

const (
	// rtpMtu is the maximum payload size per RTP packet
	rtpMtu = 1400
)

// SendFrame sends a PS frame (from MuxH264ToPS) over RTP to the platform.
//
// psData: Already-muxed PS bytes (pack header + optional PSM + PES packet)
// isKeyFrame: Whether this is a key frame (IDR)
// pts: Presentation timestamp for RTP timestamp calculation
// ssrc: Platform-provided SSRC from INVITE SDP y= field
//
// The PS data is fragmented across multiple RTP packets at MTU 1400.
// The marker bit is set to true ONLY on the last fragment of the access unit.
// Thread-safe (mutex protects sequence number increment).
//
// Returns error on UDP send failure.
func (rp *RtpPusher) SendFrame(psData []byte, isKeyFrame bool, pts time.Time, ssrc uint32) error {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	// Convert PTS to 90kHz RTP timestamp (32-bit per RFC 3550; wraps naturally)
	timestamp := uint32(timeTo90kHz(pts))

	// Fragment PS data across multiple RTP packets
	offset := 0
	totalFragments := (len(psData) + rtpMtu - 1) / rtpMtu

	for fragmentIndex := 0; offset < len(psData); fragmentIndex++ {
		end := offset + rtpMtu
		if end > len(psData) {
			end = len(psData)
		}

		fragment := psData[offset:end]
		marker := (fragmentIndex == totalFragments-1) // Last fragment

		rtpBuf, err := BuildRtpPacket(fragment, marker, ssrc, rp.seqNum, timestamp)
		if err != nil {
			return fmt.Errorf("failed to build RTP packet for fragment %d: %w", fragmentIndex, err)
		}

		// WriteToUDP on an unconnected socket is non-blocking: the packet is
		// queued to the kernel send buffer and the call returns immediately
		// (only blocks if the buffer is full, which a 1400-byte PS fragment
		// at camera bitrates never saturates).
		if _, err := rp.conn.WriteToUDP(rtpBuf, rp.remoteAddr); err != nil {
			return fmt.Errorf("failed to send RTP packet fragment %d: %w", fragmentIndex, err)
		}

		// Increment sequence number for next packet
		rp.seqNum++
		offset = end
	}

	return nil
}