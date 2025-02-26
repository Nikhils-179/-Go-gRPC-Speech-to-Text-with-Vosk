package main

import (
	"bytes"
	"context"
	"io"
	"log"
	"net"
	"net/http"

	pb_service1 "audio-texttranscript/audio-texttranscript/proto/service1"
	pb_service2 "audio-texttranscript/audio-texttranscript/proto/service2"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// gRPC Inbound: AudioProcessingService

// Our gRPC server struct
type audioProcessingServer struct {
	pb_service1.UnimplementedAudioProcessingServiceServer
}

// ProcessAudio is the gRPC method HeadUnit calls
// We forward inbound audio to Service2 via gRPC.
func (s *audioProcessingServer) ProcessAudio(stream pb_service1.AudioProcessingService_ProcessAudioServer) error {
	log.Println("Service1 (gRPC inbound): Received stream from HeadUnit")

	// Dial Service2 using gRPC
	conn, err := grpc.Dial("localhost:50052", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer conn.Close()

	service2Client := pb_service2.NewAudioToTextServiceClient(conn)
	service2Stream, err := service2Client.ConvertAudioToText(context.Background())
	if err != nil {
		return err
	}

	//In a goroutine, receive chunks from HeadUnit & forward them to Service2
	go func() {
		for {
			audioChunk, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Printf("Service1 (gRPC->gRPC): error receiving chunk from HeadUnit: %v", err)
				break
			}
			log.Println("Service1 (gRPC->gRPC): forwarding chunk to Service2 gRPC")
			service2Stream.Send(&pb_service2.AudioChunk{
				Data:      audioChunk.Data,
				SessionId: audioChunk.SessionId,
			})
		}
		service2Stream.CloseSend()
	}()

	//Relay transcriptions from Service2 back to HeadUnit
	for {
		t, err := service2Stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		log.Printf("Service1 (gRPC->gRPC): transcription from Service2: %s", t.Text)

		// Forward the transcription to HeadUnit
		stream.Send(&pb_service1.Transcription{
			Text:      t.Text,
			SessionId: t.SessionId,
		})
	}
	return nil
}

// startGRPCInbound starts the gRPC server on port 50051
func startGRPCInbound() {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Service1 gRPC inbound: failed to listen on port 50051: %v", err)
	}
	grpcServer := grpc.NewServer()
	pb_service1.RegisterAudioProcessingServiceServer(grpcServer, &audioProcessingServer{})
	log.Println("Service1 (gRPC inbound) listening on port 50051")
	grpcServer.Serve(lis)
}

// REST Inbound: /process
// If HeadUnit sends a POST to :6001/process, we forward the audio to Service2 via REST.
func processHandler(w http.ResponseWriter, r *http.Request) {
	//Read all inbound audio from HeadUnit
	audioData, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed reading body", http.StatusBadRequest)
		return
	}
	log.Printf("Service1 (REST inbound): received %d bytes from HeadUnit\n", len(audioData))

	// Single POST to Service2’s REST endpoint :6002/convert
	service2URL := "http://localhost:6002/convert"
	resp, err := http.Post(service2URL, "application/octet-stream", bytes.NewReader(audioData))
	if err != nil {
		http.Error(w, "failed contacting Service2", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// Parse Service2’s response (assuming JSON with { "transcription": "..." })
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "failed reading Service2 response", http.StatusInternalServerError)
		return
	}
	log.Println("Service1 (REST->REST): Service2 response:", string(body))

	// Return the final transcription to HeadUnit
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

// startRESTInbound starts the REST server on port 6001
func startRESTInbound() {
	http.HandleFunc("/process", processHandler)
	log.Println("Service1 (REST inbound) listening on port 6001 at /process")
	http.ListenAndServe(":6001", nil)
}

func main() {
	// Start both inbound servers concurrently
	go startGRPCInbound() // listens on :50051 for gRPC
	go startRESTInbound() // listens on :6001 for REST

	// Keep the main function alive
	select {}
}
