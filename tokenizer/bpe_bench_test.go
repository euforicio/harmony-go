package tokenizer

import (
	"strings"
	"sync"
	"testing"
)

var (
	benchCoreOnce sync.Once
	benchCore     *coreBPE
	benchCoreErr  error
)

func loadBenchCore(b *testing.B) *coreBPE {
	benchCoreOnce.Do(func() {
		pairs, err := LoadO200k()
		if err != nil {
			benchCoreErr = err
			return
		}
		benchCore, benchCoreErr = newCoreBPE(pairs, buildHarmonySpecials(), NewO200kSegmenter())
	})
	if benchCoreErr != nil {
		b.Fatalf("load core: %v", benchCoreErr)
	}
	return benchCore
}

func BenchmarkEncodePiece_Short(b *testing.B) {
	core := loadBenchCore(b)
	piece := "weather"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		toks, release := core.bytePairEncode(piece)
		if len(toks) == 0 {
			b.Fatal("expected tokens")
		}
		release()
	}
}

func BenchmarkEncodePiece_Medium(b *testing.B) {
	core := loadBenchCore(b)
	piece := "San Francisco weather forecast for the next five days with precipitation chances"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		toks, release := core.bytePairEncode(piece)
		if len(toks) == 0 {
			b.Fatal("expected tokens")
		}
		release()
	}
}

func BenchmarkEncodePiece_Large(b *testing.B) {
	core := loadBenchCore(b)
	base := "Summarise the full itinerary including breakfast, museum visits, hikes, dinner plans, and transit notes. "
	piece := strings.Repeat(base, 8)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		toks, release := core.bytePairEncode(piece)
		if len(toks) == 0 {
			b.Fatal("expected tokens")
		}
		release()
	}
}

func BenchmarkBytePairMerge(b *testing.B) {
	core := loadBenchCore(b)
	piece := strings.Repeat("tool schema requires validation ", 6)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parts, release := core.bytePairMerge(piece)
		if len(parts) == 0 {
			b.Fatal("expected parts")
		}
		release()
	}
}
