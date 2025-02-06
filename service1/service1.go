package main

import (
	"context"
	"io"
	"log"
	"net"

	pb_service1 "audio-texttranscript/audio-texttranscript/proto/service1"
	pb_service2 "audio-texttranscript/audio-texttranscript/proto/service2"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type audioProcessingServer struct {
	pb_service1.UnimplementedAudioProcessingServiceServer
}

func (s *audioProcessingServer) ProcessAudio(stream pb_service1.AudioProcessingService_ProcessAudioServer) error {
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

	go func() {
		for {
			audioChunk, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Printf("Failed to receive audio chunk: %v", err)
				break
			}
			log.Println("Forwarding audio chunk to Service2...")
			service2Stream.Send(&pb_service2.AudioChunk{
				Data:      audioChunk.Data,
				SessionId: audioChunk.SessionId,
			})
		}
		service2Stream.CloseSend()
	}()

	for {
		transcription, err := service2Stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		log.Printf("Received transcription: %s", transcription.Text)
		stream.Send(&pb_service1.Transcription{
			Text:      transcription.Text,
			SessionId: transcription.SessionId,
		})
	}

	return nil
}

func main() {
	log.Println("Starting Service1 on port 50051...")
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	pb_service1.RegisterAudioProcessingServiceServer(grpcServer, &audioProcessingServer{})
	grpcServer.Serve(lis)
}
