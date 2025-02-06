package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"

	pb_service2 "audio-texttranscript/audio-texttranscript/proto/service2"

	"google.golang.org/grpc"
)

type audioToTextServer struct {
	pb_service2.UnimplementedAudioToTextServiceServer
}

func (s *audioToTextServer) ConvertAudioToText(stream pb_service2.AudioToTextService_ConvertAudioToTextServer) error {
	var audioData []byte

	// Collect all audio chunks
	for {
		audioChunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Error receiving audio chunk: %v", err)
			return err
		}
		audioData = append(audioData, audioChunk.Data...)
	}

	// Process with Vosk
	transcription, err := transcribeAudioWithVosk(audioData)
	if err != nil {
		log.Printf("Error transcribing audio: %v", err)
		return err
	}

	log.Println("Final Transcription:", transcription)

	// Send transcription result back to the client
	return stream.Send(&pb_service2.Transcription{
		Text:      transcription,
		SessionId: "session-123",
	})
}

// Function to call Vosk for speech-to-text transcription
func transcribeAudioWithVosk(audioData []byte) (string, error) {
	// Ensure the Vosk model directory is available
	modelPath := "./vosk-model"
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return "", fmt.Errorf("vosk model not found at %s. Download and extract it first", modelPath)
	}

	// Prepare the command to run Vosk (Python)
	cmd := exec.Command("python3", "-c", `
import sys
import json
from vosk import Model, KaldiRecognizer
import wave

model = Model("vosk-model")
rec = KaldiRecognizer(model, 16000)

wav_data = sys.stdin.buffer.read()
rec.AcceptWaveform(wav_data)

result = json.loads(rec.FinalResult())
print(result["text"])
`)

	// Pass the audio data to Vosk via stdin
	cmd.Stdin = bytes.NewReader(audioData)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr

	// Run the Vosk command
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("Vosk processing error: %v", err)
	}

	// Return the transcription
	return out.String(), nil
}

func main() {
	lis, err := net.Listen("tcp", ":50052")
	if err != nil {
		log.Fatalf("Failed to listen on port 50052: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb_service2.RegisterAudioToTextServiceServer(grpcServer, &audioToTextServer{})
	log.Println("Service2 (Vosk Transcription) is running on port 50052...")

	grpcServer.Serve(lis)
}
