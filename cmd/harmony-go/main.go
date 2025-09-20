package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/euforicio/harmony-go"
)

func die(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }

func main() {
	if len(os.Args) < 2 {
		fmt.Println("harmony-go [render-msg|render-convo|render-completion|render-training|parse|decode|stop]")
		return
	}
	switch os.Args[1] {
	case "stop":
		enc, err := harmony.LoadEncoding(harmony.HarmonyGptOss)
		if err != nil {
			die(err)
		}
		toks, err := enc.StopTokens()
		if err != nil {
			die(err)
		}
		_ = json.NewEncoder(os.Stdout).Encode(toks)
	case "render-msg":
		enc, err := harmony.LoadEncoding(harmony.HarmonyGptOss)
		if err != nil {
			die(err)
		}
		var msg harmony.Message
		if err := json.NewDecoder(os.Stdin).Decode(&msg); err != nil {
			die(err)
		}
		tok, err := enc.Render(msg)
		if err != nil {
			die(err)
		}
		_ = json.NewEncoder(os.Stdout).Encode(tok)
	case "render-convo":
		enc, err := harmony.LoadEncoding(harmony.HarmonyGptOss)
		if err != nil {
			die(err)
		}
		var convo harmony.Conversation
		if err := json.NewDecoder(os.Stdin).Decode(&convo); err != nil {
			die(err)
		}
		tok, err := enc.RenderConversation(convo, nil)
		if err != nil {
			die(err)
		}
		_ = json.NewEncoder(os.Stdout).Encode(tok)
	case "render-completion":
		fs := flag.NewFlagSet("render-completion", flag.ExitOnError)
		role := fs.String("role", "assistant", "next role")
		autoDrop := fs.Bool("auto-drop", true, "auto drop analysis before final")
		_ = fs.Parse(os.Args[2:])
		enc, err := harmony.LoadEncoding(harmony.HarmonyGptOss)
		if err != nil {
			die(err)
		}
		var convo harmony.Conversation
		if err := json.NewDecoder(os.Stdin).Decode(&convo); err != nil {
			die(err)
		}
		cfg := &harmony.RenderConversationConfig{AutoDropAnalysis: *autoDrop}
		tok, err := enc.RenderConversationForCompletion(convo, harmony.Role(*role), cfg)
		if err != nil {
			die(err)
		}
		_ = json.NewEncoder(os.Stdout).Encode(tok)
	case "render-training":
		fs := flag.NewFlagSet("render-training", flag.ExitOnError)
		autoDrop := fs.Bool("auto-drop", true, "auto drop analysis before final")
		_ = fs.Parse(os.Args[2:])
		enc, err := harmony.LoadEncoding(harmony.HarmonyGptOss)
		if err != nil {
			die(err)
		}
		var convo harmony.Conversation
		if err := json.NewDecoder(os.Stdin).Decode(&convo); err != nil {
			die(err)
		}
		cfg := &harmony.RenderConversationConfig{AutoDropAnalysis: *autoDrop}
		tok, err := enc.RenderConversationForTraining(convo, cfg)
		if err != nil {
			die(err)
		}
		_ = json.NewEncoder(os.Stdout).Encode(tok)
	case "parse":
		fs := flag.NewFlagSet("parse", flag.ExitOnError)
		role := fs.String("role", "assistant", "optional starting role (user|assistant|system|developer|tool)")
		_ = fs.Parse(os.Args[2:])
		enc, err := harmony.LoadEncoding(harmony.HarmonyGptOss)
		if err != nil {
			die(err)
		}
		var tokens []uint32
		if err := json.NewDecoder(os.Stdin).Decode(&tokens); err != nil {
			die(err)
		}
		var rptr *harmony.Role
		if *role != "" {
			rr := harmony.Role(*role)
			rptr = &rr
		}
		msgs, err := enc.ParseMessagesFromCompletionTokens(tokens, rptr)
		if err != nil {
			die(err)
		}
		_ = json.NewEncoder(os.Stdout).Encode(msgs)
	case "decode":
		fs := flag.NewFlagSet("decode", flag.ExitOnError)
		if err := fs.Parse(os.Args[2:]); err != nil {
			die(err)
		}
		var tokens []uint32
		if err := json.NewDecoder(os.Stdin).Decode(&tokens); err != nil {
			die(err)
		}
		enc, err := harmony.LoadEncoding(harmony.HarmonyGptOss)
		if err != nil {
			die(err)
		}
		s, err := enc.DecodeUTF8(tokens)
		if err != nil {
			die(err)
		}
		fmt.Println(s)
	default:
		fmt.Fprintln(os.Stderr, "unimplemented")
		os.Exit(2)
	}
}
