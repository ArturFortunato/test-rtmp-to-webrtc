// +build !js

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media/samplebuilder"
)

func main() {
	rand.Seed(time.Now().UTC().UnixNano())

	//http.Handle("/", http.FileServer(http.Dir(".")))
	http.HandleFunc("/createPeerConnection", createPeerConnection)
	// http.HandleFunc("/closeConnection", onCloseByController)
	panic(http.ListenAndServe(":8080", nil))
}

func createPeerConnection(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	peerConnection, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		panic(err)
	}

	// Create a video track
	videoTrack, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: "video/vp8"}, "video", "pion")
	if err != nil {
		panic(err)
	}
	rtpSender, err := peerConnection.AddTrack(videoTrack)
	if err != nil {
		panic(err)
	}
	processRTCP(rtpSender)

	// Create a video track
	audioTrack, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: "audio/opus"}, "audio", "pion")
	if err != nil {
		panic(err)
	}
	rtpSender, err = peerConnection.AddTrack(audioTrack)
	if err != nil {
		panic(err)
	}
	processRTCP(rtpSender)

	var offer webrtc.SessionDescription

	if err := json.NewDecoder(r.Body).Decode(&offer); err != nil {
		return
	}

	if err := peerConnection.SetRemoteDescription(offer); err != nil {
		panic(err)
	}

	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		panic(err)
	} else if err = peerConnection.SetLocalDescription(answer); err != nil {
		panic(err)
	}

	response, err := json.Marshal(answer)
	if err != nil {
		panic(err)
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(response); err != nil {
		panic(err)
	}

	eventID := r.URL.Query()["eventID"][0]

	log.Println(eventID)

	go rtpToTrack(videoTrack, &codecs.VP8Packet{}, 90000, 5004)
	rtpToTrack(audioTrack, &codecs.OpusPacket{}, 48000, 5006)
	//addNewClient/(eventID, audioTrack, videoTrack)
}

// Read incoming RTCP packets
// Before these packets are retuned they are processed by interceptors. For things
// like NACK this needs to be called.
func processRTCP(rtpSender *webrtc.RTPSender) {
	go func() {
		rtcpBuf := make([]byte, 1500)

		for {
			if _, _, rtcpErr := rtpSender.Read(rtcpBuf); rtcpErr != nil {
				return
			}
		}
	}()
}

// Listen for incoming packets on a port and write them to a Track
func rtpToTrack(track *webrtc.TrackLocalStaticSample, depacketizer rtp.Depacketizer, sampleRate uint32, port int) {
	// Open a UDP Listener for RTP Packets on port 5004
	listener, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: port})
	if err != nil {
		panic(err)
	}
	defer func() {
		if err = listener.Close(); err != nil {
			panic(err)
		}
	}()

	sampleBuffer := samplebuilder.New(10, depacketizer, sampleRate)

	// Read RTP packets forever and send them to the WebRTC Client
	for {
		inboundRTPPacket := make([]byte, 1500) // UDP MTU
		packet := &rtp.Packet{}

		n, _, err := listener.ReadFrom(inboundRTPPacket)
		if err != nil {
			panic(fmt.Sprintf("error during read: %s", err))
		}

		if err = packet.Unmarshal(inboundRTPPacket[:n]); err != nil {
			panic(err)
		}

		sampleBuffer.Push(packet)
		for {
			sample := sampleBuffer.Pop()
			if sample == nil {
				break
			}

			if writeErr := track.WriteSample(*sample); writeErr != nil {
				panic(writeErr)
			}
		}
	}
}
