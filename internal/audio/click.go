package audio

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/hajimehoshi/oto/v2"
)

type clickPlayer struct {
	player oto.Player
	seeker io.Seeker
}

type ClickPool struct {
	ctx     *oto.Context
	players []clickPlayer
	next    int
}

func NewClickPool(sampleRate int, volume float64, pitchLevel int, poolSize int) (*ClickPool, error) {
	if poolSize < 1 {
		return nil, fmt.Errorf("pool size must be >= 1")
	}

	pcm := generateClickPCM(sampleRate, volume, pitchLevel)
	ctx, ready, err := oto.NewContext(sampleRate, 2, 2)
	if err != nil {
		return nil, fmt.Errorf("audio context: %w", err)
	}
	<-ready

	players := make([]clickPlayer, 0, poolSize)
	for i := 0; i < poolSize; i++ {
		reader := bytes.NewReader(pcm)
		player := ctx.NewPlayer(reader)
		seeker, ok := player.(io.Seeker)
		if !ok {
			player.Close()
			for _, cp := range players {
				cp.player.Close()
			}
			return nil, fmt.Errorf("oto player does not implement io.Seeker")
		}
		players = append(players, clickPlayer{player: player, seeker: seeker})
	}

	return &ClickPool{ctx: ctx, players: players}, nil
}

func (p *ClickPool) Play() error {
	if len(p.players) == 0 {
		return fmt.Errorf("no players available")
	}

	cp := p.players[p.next]
	p.next = (p.next + 1) % len(p.players)

	cp.player.Pause()
	cp.player.Reset()
	if _, err := cp.seeker.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind click sound failed: %w", err)
	}
	cp.player.Play()

	return nil
}

func (p *ClickPool) Close() {
	for _, cp := range p.players {
		cp.player.Close()
	}
}

func pitchFrequencies(level int) (float64, float64) {
	if level < 1 {
		level = 1
	}
	if level > 5 {
		level = 5
	}

	main := []float64{900, 1400, 2000, 2800, 3800}[level-1]
	return main, main * 0.5
}

func generateClickPCM(sampleRate int, volume float64, pitchLevel int) []byte {
	const clickDuration = 14 * time.Millisecond
	samples := int(float64(sampleRate) * clickDuration.Seconds())
	if samples < 1 {
		samples = 1
	}
	if volume < 0 {
		volume = 0
	}
	if volume > 1 {
		volume = 1
	}
	mainFreq, subFreq := pitchFrequencies(pitchLevel)

	pcm := make([]byte, samples*2*2)
	for i := 0; i < samples; i++ {
		t := float64(i) / float64(sampleRate)
		env := math.Exp(-t * 180.0)
		tone := math.Sin(2 * math.Pi * mainFreq * t)
		subTone := 0.35 * math.Sin(2*math.Pi*subFreq*t)

		s := (tone + subTone) * env * 0.95 * volume
		if s > 1 {
			s = 1
		} else if s < -1 {
			s = -1
		}

		v := int16(s * 30000)
		off := i * 4
		binary.LittleEndian.PutUint16(pcm[off:], uint16(v))
		binary.LittleEndian.PutUint16(pcm[off+2:], uint16(v))
	}

	return pcm
}
