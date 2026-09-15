package main

import (
	"context"
	"encoding/json"
	"net"
	"os"

	dictator "github.com/tyemirov/dictator/sdk/go/dictatorspeechv1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type fixture struct {
	dictator.UnimplementedVoiceServiceServer
}

func (*fixture) ListSynthesisVoices(ctx context.Context, _ *dictator.ListSynthesisVoicesRequest) (*dictator.ListSynthesisVoicesResponse, error) {
	values, _ := metadata.FromIncomingContext(ctx)
	tokens := values.Get("authorization")
	if len(tokens) != 1 || tokens[0] != "Bearer browser-dictator-token" {
		return nil, status.Error(codes.Unauthenticated, "invalid fixture token")
	}
	return &dictator.ListSynthesisVoicesResponse{Voices: []*dictator.SynthesisVoice{{VoiceId: "baya", DisplayName: "Baya", LanguageCode: "ru", SynthesisEngine: dictator.SynthesisEngine_SYNTHESIS_ENGINE_SILERO_RU, NativeSampleRateHz: []int32{48000}, DefaultSampleRateHz: 48000}}}, nil
}
func main() {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	server := grpc.NewServer()
	dictator.RegisterVoiceServiceServer(server, &fixture{})
	if err := json.NewEncoder(os.Stdout).Encode(map[string]string{"address": listener.Addr().String()}); err != nil {
		panic(err)
	}
	if err := server.Serve(listener); err != nil {
		panic(err)
	}
}
