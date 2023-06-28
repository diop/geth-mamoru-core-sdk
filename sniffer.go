package mamoru

import (
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/Mamoru-Foundation/mamoru-sniffer-go/mamoru_sniffer"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/log"
)

var (
	sniffer            *mamoru_sniffer.Sniffer
	SnifferConnectFunc = mamoru_sniffer.Connect
)

const Delta = 10 // min diff between currentBlock and highestBlock

type statusProgress interface {
	Progress() ethereum.SyncProgress
}

type Sniffer struct {
	mu     sync.Mutex
	status statusProgress
	synced bool
	delta  int64
}

func NewSniffer() *Sniffer {
	return &Sniffer{delta: Delta}
}

func (s *Sniffer) SetDownloader(downloader statusProgress) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = downloader
}

func (s *Sniffer) CheckRequirements() bool {
	return s.isSnifferEnable() && s.connect() && s.checkSynced()
}

func (s *Sniffer) checkSynced() bool {
	if s.status == nil {
		log.Info("Mamoru Sniffer status", "status", "nil")
		return false
	}

	progress := s.status.Progress()

	log.Info("Mamoru Sniffer sync", "syncing", s.synced, "diff", int64(progress.HighestBlock)-int64(progress.CurrentBlock))

	if progress.CurrentBlock < progress.HighestBlock {
		s.synced = false
	}
	if s.synced {
		return true
	}

	if progress.CurrentBlock > 0 && progress.HighestBlock > 0 {
		if int64(progress.HighestBlock)-int64(progress.CurrentBlock) <= s.delta {
			s.synced = true
		}
		log.Info("Mamoru Sniffer sync", "syncing", s.synced, "current", int64(progress.CurrentBlock), "highest", int64(progress.HighestBlock))
		return s.synced
	}

	return false
}

func (s *Sniffer) isSnifferEnable() bool {
	val, ok := os.LookupEnv("MAMORU_SNIFFER_ENABLE")
	if !ok {
		return false
	}

	isEnable, err := strconv.ParseBool(val)
	if err != nil {
		log.Error("Mamoru Sniffer env parse error", "err", err)
		return false
	}

	return isEnable
}

func (s *Sniffer) connect() bool {
	if sniffer != nil {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var err error
	if sniffer == nil {
		sniffer, err = SnifferConnectFunc()
		if err != nil {
			erst := strings.Replace(err.Error(), "\t", "", -1)
			erst = strings.Replace(erst, "\n", "", -1)
			//	erst = strings.Replace(erst, " ", "", -1)
			log.Error("Mamoru Sniffer connect", "err", erst)
			return false
		}
	}

	return true
}
