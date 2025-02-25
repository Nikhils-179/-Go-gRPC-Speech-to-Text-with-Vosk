package main

import (
	"bytes"
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/gordonklaus/portaudio"

	pb_headunit "audio-texttranscript/audio-texttranscript/proto/headunit"
	pb_service1 "audio-texttranscript/audio-texttranscript/proto/service1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
)

const (
    sampleRate = 16000// use a standard sample rate for microphone input
    chunkSize  = 8192  // try a larger buffer size
)

// int16SliceToBytes converts a slice of int16 PCM samples to a byte slice (little-endian).
func int16SliceToBytes(data []int16) []byte {
	bytes := make([]byte, len(data)*2)
	for i, sample := range data {
		bytes[i*2] = byte(sample)
		bytes[i*2+1] = byte(sample >> 8)
	}
	return bytes
}

// ---------- gRPC Implementation for HeadUnit ----------

type headUnitServer struct {
	pb_headunit.UnimplementedHeadUnitServiceServer
}

// StreamAudio is invoked when a gRPC client calls the method.
// It captures audio from the default microphone and sends the data to Service1 via gRPC.
func (s *headUnitServer) StreamAudio(stream pb_headunit.HeadUnitService_StreamAudioServer) error {
	// Connect to Service1's gRPC service on port 50051.
	conn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	log.Println("HeadUnit gRPC: StreamAudio started")
	if err != nil {
		return err
	}
	defer conn.Close()

	service1Client := pb_service1.NewAudioProcessingServiceClient(conn)
	service1Stream, err := service1Client.ProcessAudio(context.Background())
	if err != nil {
		return err
	}

	// Initialize PortAudio.
	if err := portaudio.Initialize(); err != nil {
		return err
	}
	defer portaudio.Terminate()

	// Open the default input stream (mono, sampleRate 16000).
	buffer := make([]int16, 512)
	streamIn, err := portaudio.OpenDefaultStream(1, 0, sampleRate, len(buffer), buffer)
	if err != nil {
		return err
	}
	defer streamIn.Close()

	if err := streamIn.Start(); err != nil {
		return err
	}
	defer streamIn.Stop()

	log.Println("HeadUnit gRPC: Recording audio from microphone... Please speak now.")
	// Give the user time to speak.
	stopTime := time.Now().Add(5 * time.Second)
	for time.Now().Before(stopTime) {
		err := streamIn.Read()
		if err != nil {
			// If overflow error, log and continue rather than break.
			if err.Error() == "Input overflowed" {
				log.Printf("Warning: %v", err)
				break
			}
			log.Printf("Error reading microphone input: %v", err)
			break
		}
		err = service1Stream.Send(&pb_service1.AudioChunk{
			Data:      int16SliceToBytes(buffer),
			SessionId: "session-123",
		})
		if err != nil {
			log.Printf("Error sending audio chunk: %v", err)
			break
		}
	}

	log.Println("HeadUnit gRPC: Audio streaming stopped.")
	service1Stream.CloseSend()

	// Receive and log transcriptions from Service1.
	for {
		transcription, err := service1Stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("HeadUnit gRPC: Error receiving transcription: %v", err)
			break
		}
		log.Printf("HeadUnit gRPC: Final Transcription: %s", transcription.Text)
	}

	return nil
}

// ---------- REST Endpoint for HeadUnit ----------

// restSendAudioHandler captures audio from the microphone using PortAudio,
// then POSTs the raw PCM data to Service1's REST endpoint.
func restSendAudioHandler(w http.ResponseWriter, r *http.Request) {
	// Initialize PortAudio
	if err := portaudio.Initialize(); err != nil {
		http.Error(w, "failed to initialize PortAudio", http.StatusInternalServerError)
		return
	}
	defer portaudio.Terminate()

	// Open default input stream: 1 channel, sampleRate 16000.
	buffer := make([]int16, 4096) // Increased buffer size
	streamIn, err := portaudio.OpenDefaultStream(1, 0, sampleRate, len(buffer), buffer)
	if err != nil {
		http.Error(w, "failed to open stream", http.StatusInternalServerError)
		return
	}
	defer streamIn.Close()

	if err := streamIn.Start(); err != nil {
		http.Error(w, "failed to start stream", http.StatusInternalServerError)
		return
	}
	defer streamIn.Stop()

	log.Println("HeadUnit REST: Recording audio from microphone... Please speak now.")

	// Use buffered channels for better concurrency.
	audioCh := make(chan []byte, 500) // Increased buffer to avoid overflow
	done := make(chan struct{})

	// Goroutine to read audio continuously
	go func() {
		for {
			select {
			case <-done:
				close(audioCh)
				return
			default:
				err := streamIn.Read()
				if err != nil {
					if err.Error() == "Input overflowed" {
						log.Println("Warning: Input overflowed. Skipping this chunk.")
						continue
					}
					log.Printf("Error reading microphone input: %v", err)
					return
				}
				audioCh <- int16SliceToBytes(buffer)
			}
		}
	}()

	// Send audio chunks in parallel without blocking recording.
	service1URL := "http://localhost:6001/process"
	client := &http.Client{}
	audioData := new(bytes.Buffer)

	// Capture audio for a fixed duration
	stopTime := time.Now().Add(5 * time.Second)
	for time.Now().Before(stopTime) {
		select {
		case chunk := <-audioCh:
			audioData.Write(chunk)
			// Send in smaller chunks to avoid delays
			if audioData.Len() > 1024*5 { 
				go func(data []byte) {
					resp, err := client.Post(service1URL, "application/octet-stream", bytes.NewReader(data))
					if err != nil {
						log.Println("Warning: Failed to send chunk to Service1", err)
						return
					}
					resp.Body.Close()
				}(audioData.Bytes())
				audioData.Reset()
			}
		case <-time.After(50 * time.Millisecond): 
		}
	}

	// Final transmission
	if audioData.Len() > 0 {
		resp, err := client.Post(service1URL, "application/octet-stream", bytes.NewReader(audioData.Bytes()))
		if err != nil {
			http.Error(w, "failed to forward final audio to Service1", http.StatusInternalServerError)
			return
		}
		resp.Body.Close()
	}

	// Notify Goroutine to stop
	close(done)

	// Read Service1's response
	resp, err := client.Get(service1URL + "/result")
	if err != nil {
		http.Error(w, "failed to read response from Service1", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "failed to read response body from Service1", http.StatusInternalServerError)
		return
	}

	log.Println("HeadUnit REST: Received transcription:", string(body))
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

// startHeadUnitGRPCServer starts the gRPC server on port 50050.
func startHeadUnitGRPCServer() {
	lis, err := net.Listen("tcp", ":50050")
	if err != nil {
		log.Fatalf("HeadUnit: failed to listen on port 50050: %v", err)
	}
	grpcServer := grpc.NewServer()
	pb_headunit.RegisterHeadUnitServiceServer(grpcServer, &headUnitServer{})
	reflection.Register(grpcServer)
	log.Println("HeadUnit gRPC service running on port 50050...")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("HeadUnit: gRPC serve error: %v", err)
	}
}

// startHeadUnitRESTServer starts the REST server on port 7000.
func startHeadUnitRESTServer() {
	http.HandleFunc("/sendAudio", restSendAudioHandler)
	log.Println("HeadUnit REST endpoint running on port 7002 at /sendAudio...")
	log.Fatal(http.ListenAndServe(":7002", nil))
	
}

// runClient simulates a gRPC client that connects to HeadUnit's gRPC endpoint.
func runClient() {
	conn, err := grpc.Dial("localhost:50050", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to HeadUnit service: %v", err)
	}
	defer conn.Close()

	client := pb_headunit.NewHeadUnitServiceClient(conn)
	stream, err := client.StreamAudio(context.Background())
	if err != nil {
		log.Fatalf("Failed to start audio stream: %v", err)
	}

	for {
		transcription, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("Error receiving transcription: %v", err)
		}
		log.Printf("Client received transcription: %s", transcription.Text)
	}
}

func main() {
	// Start the gRPC and REST servers concurrently.
	//go startHeadUnitGRPCServer()
	go startHeadUnitRESTServer()

	// Wait briefly for the servers to start.
	time.Sleep(2 * time.Second)
	//runClient()
	
	select{}

}
