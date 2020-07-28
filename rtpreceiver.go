// +build !js

package webrtc

import (
	"fmt"
	"io"
	"sync"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

// RTPReceiver allows an application to inspect the receipt of a Track
type RTPReceiver struct {
	kind      RTPCodecType
	transport Transport

	track *Track

	closed, received chan interface{}
	mu               sync.RWMutex

	rtpReadStream  rtp.ReadStream
	rtcpReadStream rtcp.ReadStream

	// A reference to the associated api object
	api *API
}

// NewRTPReceiver constructs a new RTPReceiver
func (api *API) NewRTPReceiver(kind RTPCodecType, transport Transport) (*RTPReceiver, error) {
	if transport == nil {
		return nil, fmt.Errorf("DTLSTransport must not be nil")
	}

	return &RTPReceiver{
		kind:      kind,
		transport: transport,
		api:       api,
		closed:    make(chan interface{}),
		received:  make(chan interface{}),
	}, nil
}

// Transport returns the currently-configured *DTLSTransport or nil
// if one has not yet been configured
func (r *RTPReceiver) Transport() Transport {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.transport
}

// Track returns the RTCRtpTransceiver track
func (r *RTPReceiver) Track() *Track {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.track
}

// Receive initialize the track and starts all the transports
func (r *RTPReceiver) Receive(parameters RTPReceiveParameters) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	select {
	case <-r.received:
		return fmt.Errorf("Receive has already been called")
	default:
	}
	defer close(r.received)

	rtpSession, err := r.transport.RTPSession()
	if err != nil {
		return err
	}

	ssrc := parameters.Encodings.SSRC
	if ssrc > 0 {
		r.rtpReadStream, err = rtpSession.OpenReadStream(parameters.Encodings.SSRC)
	} else {
		r.rtpReadStream, ssrc, err = rtpSession.AcceptStream()
	}

	if err != nil {
		return err
	}

	rtcpSession, err := r.transport.RTCPSession()
	if err != nil {
		return err
	}

	r.rtcpReadStream, err = rtcpSession.OpenReadStream(ssrc)
	if err != nil {
		return err
	}

	r.track = &Track{
		kind:     r.kind,
		ssrc:     ssrc,
		receiver: r,
	}

	return nil
}

// Read reads incoming RTCP for this RTPReceiver
func (r *RTPReceiver) Read(b []byte) (n int, err error) {
	select {
	case <-r.received:
		return r.rtcpReadStream.Read(b)
	case <-r.closed:
		return 0, io.ErrClosedPipe
	}
}

// ReadRTCP is a convenience method that wraps Read and unmarshals for you
func (r *RTPReceiver) ReadRTCP() ([]rtcp.Packet, error) {
	b := make([]byte, receiveMTU)
	i, err := r.Read(b)
	if err != nil {
		return nil, err
	}

	return rtcp.Unmarshal(b[:i])
}

func (r *RTPReceiver) haveReceived() bool {
	select {
	case <-r.received:
		return true
	default:
		return false
	}
}

// Stop irreversibly stops the RTPReceiver
func (r *RTPReceiver) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	select {
	case <-r.closed:
		return nil
	default:
	}

	select {
	case <-r.received:
		if r.rtcpReadStream != nil {
			if err := r.rtcpReadStream.Close(); err != nil {
				return err
			}
		}
		if r.rtpReadStream != nil {
			if err := r.rtpReadStream.Close(); err != nil {
				return err
			}
		}
	default:
	}

	close(r.closed)
	return nil
}

// readRTP should only be called by a track, this only exists so we can keep state in one place
func (r *RTPReceiver) readRTP(b []byte) (n int, err error) {
	<-r.received
	return r.rtpReadStream.Read(b)
}
