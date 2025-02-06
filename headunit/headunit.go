package main

import (
	"context"
	"io"
	"log"
	"net"
	"time"

	"github.com/gordonklaus/portaudio"

	pb_headunit "audio-texttranscript/audio-texttranscript/proto/headunit"
	pb_service1 "audio-texttranscript/audio-texttranscript/proto/service1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
)

const (
	sampleRate = 16000 
	chunkSize  = 1024  
)

type headUnitServer struct {
	pb_headunit.UnimplementedHeadUnitServiceServer
}

func (s *headUnitServer) StreamAudio(stream pb_headunit.HeadUnitService_StreamAudioServer) error {
	conn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	log.Println("StreamAudio started on HeadUnit service")

	if err != nil {
		return err
	}
	defer conn.Close()

	service1Client := pb_service1.NewAudioProcessingServiceClient(conn)
	service1Stream, err := service1Client.ProcessAudio(context.Background())
	if err != nil {
		return err
	}

	// Start microphone recording
	portaudio.Initialize()
	defer portaudio.Terminate()

	buffer := make([]int16, chunkSize)
	streamIn, err := portaudio.OpenDefaultStream(1, 0, sampleRate, len(buffer), buffer)
	if err != nil {
		return err
	}
	defer streamIn.Close()

	err = streamIn.Start()
	if err != nil {
		return err
	}
	defer streamIn.Stop()

	log.Println("Recording audio from microphone...")

	stopTime := time.Now().Add(5 * time.Second)

	for time.Now().Before(stopTime) {
		err := streamIn.Read()
		if err != nil {
			log.Printf("Error reading microphone input: %v", err)
			break
		}

		// Send real audio data to Service1
		err = service1Stream.Send(&pb_service1.AudioChunk{
			Data:      int16SliceToBytes(buffer),
			SessionId: "session-123",
		})
		if err != nil {
			log.Printf("Error sending audio chunk: %v", err)
			break
		}
	}

	log.Println("Audio streaming stopped.")
	service1Stream.CloseSend()

	for {
		transcription, err := service1Stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Error receiving transcription: %v", err)
			break
		}
		log.Printf("Final Transcription: %s", transcription.Text)
	}

	return nil
}

// Convert int16 PCM audio data to bytes for gRPC transmission
func int16SliceToBytes(data []int16) []byte {
	bytes := make([]byte, len(data)*2)
	for i, sample := range data {
		bytes[i*2] = byte(sample)
		bytes[i*2+1] = byte(sample >> 8)
	}
	return bytes
}

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
		log.Printf("Received transcription: %s", transcription.Text)
	}
}

func main() {
	lis, err := net.Listen("tcp", ":50050")
	if err != nil {
		log.Fatalf("Failed to listen on port 50050: %v", err)
	}
	grpcServer := grpc.NewServer()
	pb_headunit.RegisterHeadUnitServiceServer(grpcServer, &headUnitServer{})
	reflection.Register(grpcServer)
	log.Println("HeadUnit service is running on port 50050...")

	// Start gRPC server in a goroutine
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to start HeadUnit gRPC server: %v", err)
		}
	}()

	// **Wait for the server to start before calling the client**
	time.Sleep(2 * time.Second)

	// **Automatically call runClient() to start capturing and processing audio**
	runClient()
}
