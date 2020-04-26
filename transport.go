package webrtc

import (
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

type Transport interface {
	RTPSession() (rtp.Session, error)
	RTCPSession() (rtcp.Session, error)
}
