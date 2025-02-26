package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"

	pb_service2 "audio-texttranscript/audio-texttranscript/proto/service2"

	"google.golang.org/grpc"
)

// audioToTextServer implements the gRPC interface for audio transcription.
type audioToTextServer struct {
	pb_service2.UnimplementedAudioToTextServiceServer
}

// ConvertAudioToText receives audio via a gRPC stream, concatenates the data, and transcribes it with Vosk.
func (s *audioToTextServer) ConvertAudioToText(stream pb_service2.AudioToTextService_ConvertAudioToTextServer) error {
	var audioData []byte

	// Gather all audio chunks from the gRPC stream
	for {
		audioChunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Service2 (gRPC): error receiving audio chunk: %v", err)
			return err
		}
		audioData = append(audioData, audioChunk.Data...)
	}

	// Transcribe with Vosk
	transcription, err := transcribeAudioWithVosk(audioData)
	if err != nil {
		log.Printf("Service2 (gRPC): error transcribing audio: %v", err)
		return err
	}

	log.Println("Service2 (gRPC): final transcription:", transcription)

	// Send the transcription back over gRPC
	return stream.Send(&pb_service2.Transcription{
		Text:      transcription,
		SessionId: "session-123",
	})
}

// transcribeAudioWithVosk runs a Python snippet that uses vosk to transcribe PCM data
func transcribeAudioWithVosk(audioData []byte) (string, error) {
	modelPath := "./vosk-model"
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return "", fmt.Errorf("vosk model not found at %s. Download and extract it first", modelPath)
	}

	cmd := exec.Command("python3", "-c", `
import sys, json
from vosk import Model, KaldiRecognizer
model = Model("vosk-model")
rec = KaldiRecognizer(model, 16000)
wav_data = sys.stdin.buffer.read()
rec.AcceptWaveform(wav_data)
result = json.loads(rec.FinalResult())
print(result["text"])
`)
	cmd.Stdin = bytes.NewReader(audioData)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("Vosk processing error: %v", err)
	}
	return out.String(), nil
}

// gRPC server on port :50052
func startGRPCServer() {
	lis, err := net.Listen("tcp", ":50052")
	if err != nil {
		log.Fatalf("Service2 (gRPC) failed to listen on port 50052: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb_service2.RegisterAudioToTextServiceServer(grpcServer, &audioToTextServer{})
	log.Println("Service2 (gRPC) listening on port 50052")
	grpcServer.Serve(lis)
}

// REST endpoint (/convert) on port :6002
func startRESTServer() {
	http.HandleFunc("/convert", convertHandler)
	log.Println("Service2 (REST) listening on port 6002 at /convert")
	log.Fatal(http.ListenAndServe(":6002", nil))
}

// convertHandler handles a POST with raw audio data, transcribes it, and returns JSON: { "transcription": "..." }
func convertHandler(w http.ResponseWriter, r *http.Request) {
	audioData, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	transcription, err := transcribeAudioWithVosk(audioData)
	if err != nil {
		http.Error(w, fmt.Sprintf("vosk error: %v", err), http.StatusInternalServerError)
		return
	}
	// Return JSON
	resp := map[string]string{"transcription": transcription}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func main() {
	go startGRPCServer()
	go startRESTServer()
	select {}
}
