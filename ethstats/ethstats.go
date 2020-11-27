
















package ethstats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/les"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/miner"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/gorilla/websocket"
)

const (
	
	
	historyUpdateRange = 50

	
	
	txChanSize = 4096
	
	chainHeadChanSize = 10
)


type backend interface {
	SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription
	SubscribeNewTxsEvent(ch chan<- core.NewTxsEvent) event.Subscription
	CurrentHeader() *types.Header
	HeaderByNumber(ctx context.Context, number rpc.BlockNumber) (*types.Header, error)
	GetTd(ctx context.Context, hash common.Hash) *big.Int
	Stats() (pending int, queued int)
	Downloader() *downloader.Downloader
}



type fullNodeBackend interface {
	backend
	Miner() *miner.Miner
	BlockByNumber(ctx context.Context, number rpc.BlockNumber) (*types.Block, error)
	CurrentBlock() *types.Block
	SuggestPrice(ctx context.Context) (*big.Int, error)
}



type Service struct {
	server  *p2p.Server 
	backend backend
	engine  consensus.Engine 

	node string 
	pass string 
	host string 

	pongCh chan struct{} 
	histCh chan []uint64 

}












type connWrapper struct {
	conn *websocket.Conn

	rlock sync.Mutex
	wlock sync.Mutex
}

func newConnectionWrapper(conn *websocket.Conn) *connWrapper {
	return &connWrapper{conn: conn}
}


func (w *connWrapper) WriteJSON(v interface{}) error {
	w.wlock.Lock()
	defer w.wlock.Unlock()

	return w.conn.WriteJSON(v)
}


func (w *connWrapper) ReadJSON(v interface{}) error {
	w.rlock.Lock()
	defer w.rlock.Unlock()

	return w.conn.ReadJSON(v)
}


func (w *connWrapper) Close() error {
	
	
	return w.conn.Close()
}


func New(node *node.Node, backend backend, engine consensus.Engine, url string) error {
	
	re := regexp.MustCompile("([^:@]*)(:([^@]*))?@(.+)")
	parts := re.FindStringSubmatch(url)
	if len(parts) != 5 {
		return fmt.Errorf("invalid netstats url: \"%s\", should be nodename:secret@host:port", url)
	}
	ethstats := &Service{
		backend: backend,
		engine:  engine,
		server:  node.Server(),
		node:    parts[1],
		pass:    parts[3],
		host:    parts[4],
		pongCh:  make(chan struct{}),
		histCh:  make(chan []uint64, 1),
	}

	node.RegisterLifecycle(ethstats)
	return nil
}


func (s *Service) Start() error {
	go s.loop()

	log.Info("Stats daemon started")
	return nil
}


func (s *Service) Stop() error {
	log.Info("Stats daemon stopped")
	return nil
}



func (s *Service) loop() {
	
	chainHeadCh := make(chan core.ChainHeadEvent, chainHeadChanSize)
	headSub := s.backend.SubscribeChainHeadEvent(chainHeadCh)
	defer headSub.Unsubscribe()

	txEventCh := make(chan core.NewTxsEvent, txChanSize)
	txSub := s.backend.SubscribeNewTxsEvent(txEventCh)
	defer txSub.Unsubscribe()

	
	var (
		quitCh = make(chan struct{})
		headCh = make(chan *types.Block, 1)
		txCh   = make(chan struct{}, 1)
	)
	go func() {
		var lastTx mclock.AbsTime

	HandleLoop:
		for {
			select {
			
			case head := <-chainHeadCh:
				select {
				case headCh <- head.Block:
				default:
				}

			
			case <-txEventCh:
				if time.Duration(mclock.Now()-lastTx) < time.Second {
					continue
				}
				lastTx = mclock.Now()

				select {
				case txCh <- struct{}{}:
				default:
				}

			
			case <-txSub.Err():
				break HandleLoop
			case <-headSub.Err():
				break HandleLoop
			}
		}
		close(quitCh)
	}()

	
	path := fmt.Sprintf("%s/api", s.host)
	urls := []string{path}

	
	if !strings.Contains(path, ":
		urls = []string{"wss:
	}

	errTimer := time.NewTimer(0)
	defer errTimer.Stop()
	
	for {
		select {
		case <-quitCh:
			return
		case <-errTimer.C:
			
			var (
				conn *connWrapper
				err  error
			)
			dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
			header := make(http.Header)
			header.Set("origin", "http:
			for _, url := range urls {
				c, _, e := dialer.Dial(url, header)
				err = e
				if err == nil {
					conn = newConnectionWrapper(c)
					break
				}
			}
			if err != nil {
				log.Warn("Stats server unreachable", "err", err)
				errTimer.Reset(10 * time.Second)
				continue
			}
			
			if err = s.login(conn); err != nil {
				log.Warn("Stats login failed", "err", err)
				conn.Close()
				errTimer.Reset(10 * time.Second)
				continue
			}
			go s.readLoop(conn)

			
			if err = s.report(conn); err != nil {
				log.Warn("Initial stats report failed", "err", err)
				conn.Close()
				errTimer.Reset(0)
				continue
			}
			
			fullReport := time.NewTicker(15 * time.Second)

			for err == nil {
				select {
				case <-quitCh:
					fullReport.Stop()
					
					conn.Close()
					return

				case <-fullReport.C:
					if err = s.report(conn); err != nil {
						log.Warn("Full stats report failed", "err", err)
					}
				case list := <-s.histCh:
					if err = s.reportHistory(conn, list); err != nil {
						log.Warn("Requested history report failed", "err", err)
					}
				case head := <-headCh:
					if err = s.reportBlock(conn, head); err != nil {
						log.Warn("Block stats report failed", "err", err)
					}
					if err = s.reportPending(conn); err != nil {
						log.Warn("Post-block transaction stats report failed", "err", err)
					}
				case <-txCh:
					if err = s.reportPending(conn); err != nil {
						log.Warn("Transaction stats report failed", "err", err)
					}
				}
			}
			fullReport.Stop()

			
			conn.Close()
			errTimer.Reset(0)
		}
	}
}





func (s *Service) readLoop(conn *connWrapper) {
	
	defer conn.Close()

	for {
		
		var blob json.RawMessage
		if err := conn.ReadJSON(&blob); err != nil {
			log.Warn("Failed to retrieve stats server message", "err", err)
			return
		}
		
		var ping string
		if err := json.Unmarshal(blob, &ping); err == nil && strings.HasPrefix(ping, "primus::ping::") {
			if err := conn.WriteJSON(strings.Replace(ping, "ping", "pong", -1)); err != nil {
				log.Warn("Failed to respond to system ping message", "err", err)
				return
			}
			continue
		}
		
		var msg map[string][]interface{}
		if err := json.Unmarshal(blob, &msg); err != nil {
			log.Warn("Failed to decode stats server message", "err", err)
			return
		}
		log.Trace("Received message from stats server", "msg", msg)
		if len(msg["emit"]) == 0 {
			log.Warn("Stats server sent non-broadcast", "msg", msg)
			return
		}
		command, ok := msg["emit"][0].(string)
		if !ok {
			log.Warn("Invalid stats server message type", "type", msg["emit"][0])
			return
		}
		
		if len(msg["emit"]) == 2 && command == "node-pong" {
			select {
			case s.pongCh <- struct{}{}:
				
				continue
			default:
				
				log.Warn("Stats server pinger seems to have died")
				return
			}
		}
		
		if len(msg["emit"]) == 2 && command == "history" {
			
			request, ok := msg["emit"][1].(map[string]interface{})
			if !ok {
				log.Warn("Invalid stats history request", "msg", msg["emit"][1])
				select {
				case s.histCh <- nil: 
				default:
				}
				continue
			}
			list, ok := request["list"].([]interface{})
			if !ok {
				log.Warn("Invalid stats history block list", "list", request["list"])
				return
			}
			
			numbers := make([]uint64, len(list))
			for i, num := range list {
				n, ok := num.(float64)
				if !ok {
					log.Warn("Invalid stats history block number", "number", num)
					return
				}
				numbers[i] = uint64(n)
			}
			select {
			case s.histCh <- numbers:
				continue
			default:
			}
		}
		
		log.Info("Unknown stats message", "msg", msg)
	}
}



type nodeInfo struct {
	Name     string `json:"name"`
	Node     string `json:"node"`
	Port     int    `json:"port"`
	Network  string `json:"net"`
	Protocol string `json:"protocol"`
	API      string `json:"api"`
	Os       string `json:"os"`
	OsVer    string `json:"os_v"`
	Client   string `json:"client"`
	History  bool   `json:"canUpdateHistory"`
}


type authMsg struct {
	ID     string   `json:"id"`
	Info   nodeInfo `json:"info"`
	Secret string   `json:"secret"`
}


func (s *Service) login(conn *connWrapper) error {
	
	infos := s.server.NodeInfo()

	var network, protocol string
	if info := infos.Protocols["eth"]; info != nil {
		network = fmt.Sprintf("%d", info.(*eth.NodeInfo).Network)
		protocol = fmt.Sprintf("eth/%d", eth.ProtocolVersions[0])
	} else {
		network = fmt.Sprintf("%d", infos.Protocols["les"].(*les.NodeInfo).Network)
		protocol = fmt.Sprintf("les/%d", les.ClientProtocolVersions[0])
	}
	auth := &authMsg{
		ID: s.node,
		Info: nodeInfo{
			Name:     s.node,
			Node:     infos.Name,
			Port:     infos.Ports.Listener,
			Network:  network,
			Protocol: protocol,
			API:      "No",
			Os:       runtime.GOOS,
			OsVer:    runtime.GOARCH,
			Client:   "0.1.1",
			History:  true,
		},
		Secret: s.pass,
	}
	login := map[string][]interface{}{
		"emit": {"hello", auth},
	}
	if err := conn.WriteJSON(login); err != nil {
		return err
	}
	
	var ack map[string][]string
	if err := conn.ReadJSON(&ack); err != nil || len(ack["emit"]) != 1 || ack["emit"][0] != "ready" {
		return errors.New("unauthorized")
	}
	return nil
}




func (s *Service) report(conn *connWrapper) error {
	if err := s.reportLatency(conn); err != nil {
		return err
	}
	if err := s.reportBlock(conn, nil); err != nil {
		return err
	}
	if err := s.reportPending(conn); err != nil {
		return err
	}
	if err := s.reportStats(conn); err != nil {
		return err
	}
	return nil
}



func (s *Service) reportLatency(conn *connWrapper) error {
	
	start := time.Now()

	ping := map[string][]interface{}{
		"emit": {"node-ping", map[string]string{
			"id":         s.node,
			"clientTime": start.String(),
		}},
	}
	if err := conn.WriteJSON(ping); err != nil {
		return err
	}
	
	select {
	case <-s.pongCh:
		
	case <-time.After(5 * time.Second):
		
		return errors.New("ping timed out")
	}
	latency := strconv.Itoa(int((time.Since(start) / time.Duration(2)).Nanoseconds() / 1000000))

	
	log.Trace("Sending measured latency to ethstats", "latency", latency)

	stats := map[string][]interface{}{
		"emit": {"latency", map[string]string{
			"id":      s.node,
			"latency": latency,
		}},
	}
	return conn.WriteJSON(stats)
}


type blockStats struct {
	Number     *big.Int       `json:"number"`
	Hash       common.Hash    `json:"hash"`
	ParentHash common.Hash    `json:"parentHash"`
	Timestamp  *big.Int       `json:"timestamp"`
	Miner      common.Address `json:"miner"`
	GasUsed    uint64         `json:"gasUsed"`
	GasLimit   uint64         `json:"gasLimit"`
	Diff       string         `json:"difficulty"`
	TotalDiff  string         `json:"totalDifficulty"`
	Txs        []txStats      `json:"transactions"`
	TxHash     common.Hash    `json:"transactionsRoot"`
	Root       common.Hash    `json:"stateRoot"`
	Uncles     uncleStats     `json:"uncles"`
}


type txStats struct {
	Hash common.Hash `json:"hash"`
}



type uncleStats []*types.Header

func (s uncleStats) MarshalJSON() ([]byte, error) {
	if uncles := ([]*types.Header)(s); len(uncles) > 0 {
		return json.Marshal(uncles)
	}
	return []byte("[]"), nil
}


func (s *Service) reportBlock(conn *connWrapper, block *types.Block) error {
	
	details := s.assembleBlockStats(block)

	
	log.Trace("Sending new block to ethstats", "number", details.Number, "hash", details.Hash)

	stats := map[string]interface{}{
		"id":    s.node,
		"block": details,
	}
	report := map[string][]interface{}{
		"emit": {"block", stats},
	}
	return conn.WriteJSON(report)
}



func (s *Service) assembleBlockStats(block *types.Block) *blockStats {
	
	var (
		header *types.Header
		td     *big.Int
		txs    []txStats
		uncles []*types.Header
	)

	
	fullBackend, ok := s.backend.(fullNodeBackend)
	if ok {
		if block == nil {
			block = fullBackend.CurrentBlock()
		}
		header = block.Header()
		td = fullBackend.GetTd(context.Background(), header.Hash())

		txs = make([]txStats, len(block.Transactions()))
		for i, tx := range block.Transactions() {
			txs[i].Hash = tx.Hash()
		}
		uncles = block.Uncles()
	} else {
		
		if block != nil {
			header = block.Header()
		} else {
			header = s.backend.CurrentHeader()
		}
		td = s.backend.GetTd(context.Background(), header.Hash())
		txs = []txStats{}
	}

	
	author, _ := s.engine.Author(header)

	return &blockStats{
		Number:     header.Number,
		Hash:       header.Hash(),
		ParentHash: header.ParentHash,
		Timestamp:  new(big.Int).SetUint64(header.Time),
		Miner:      author,
		GasUsed:    header.GasUsed,
		GasLimit:   header.GasLimit,
		Diff:       header.Difficulty.String(),
		TotalDiff:  td.String(),
		Txs:        txs,
		TxHash:     header.TxHash,
		Root:       header.Root,
		Uncles:     uncles,
	}
}



func (s *Service) reportHistory(conn *connWrapper, list []uint64) error {
	
	indexes := make([]uint64, 0, historyUpdateRange)
	if len(list) > 0 {
		
		indexes = append(indexes, list...)
	} else {
		
		head := s.backend.CurrentHeader().Number.Int64()
		start := head - historyUpdateRange + 1
		if start < 0 {
			start = 0
		}
		for i := uint64(start); i <= uint64(head); i++ {
			indexes = append(indexes, i)
		}
	}
	
	history := make([]*blockStats, len(indexes))
	for i, number := range indexes {
		fullBackend, ok := s.backend.(fullNodeBackend)
		
		var block *types.Block
		if ok {
			block, _ = fullBackend.BlockByNumber(context.Background(), rpc.BlockNumber(number)) 
		} else {
			if header, _ := s.backend.HeaderByNumber(context.Background(), rpc.BlockNumber(number)); header != nil {
				block = types.NewBlockWithHeader(header)
			}
		}
		
		if block != nil {
			history[len(history)-1-i] = s.assembleBlockStats(block)
			continue
		}
		
		history = history[len(history)-i:]
		break
	}
	
	if len(history) > 0 {
		log.Trace("Sending historical blocks to ethstats", "first", history[0].Number, "last", history[len(history)-1].Number)
	} else {
		log.Trace("No history to send to stats server")
	}
	stats := map[string]interface{}{
		"id":      s.node,
		"history": history,
	}
	report := map[string][]interface{}{
		"emit": {"history", stats},
	}
	return conn.WriteJSON(report)
}


type pendStats struct {
	Pending int `json:"pending"`
}



func (s *Service) reportPending(conn *connWrapper) error {
	
	pending, _ := s.backend.Stats()
	
	log.Trace("Sending pending transactions to ethstats", "count", pending)

	stats := map[string]interface{}{
		"id": s.node,
		"stats": &pendStats{
			Pending: pending,
		},
	}
	report := map[string][]interface{}{
		"emit": {"pending", stats},
	}
	return conn.WriteJSON(report)
}


type nodeStats struct {
	Active   bool `json:"active"`
	Syncing  bool `json:"syncing"`
	Mining   bool `json:"mining"`
	Hashrate int  `json:"hashrate"`
	Peers    int  `json:"peers"`
	GasPrice int  `json:"gasPrice"`
	Uptime   int  `json:"uptime"`
}



func (s *Service) reportStats(conn *connWrapper) error {
	
	var (
		mining   bool
		hashrate int
		syncing  bool
		gasprice int
	)
	
	fullBackend, ok := s.backend.(fullNodeBackend)
	if ok {
		mining = fullBackend.Miner().Mining()
		hashrate = int(fullBackend.Miner().HashRate())

		sync := fullBackend.Downloader().Progress()
		syncing = fullBackend.CurrentHeader().Number.Uint64() >= sync.HighestBlock

		price, _ := fullBackend.SuggestPrice(context.Background())
		gasprice = int(price.Uint64())
	} else {
		sync := s.backend.Downloader().Progress()
		syncing = s.backend.CurrentHeader().Number.Uint64() >= sync.HighestBlock
	}
	
	log.Trace("Sending node details to ethstats")

	stats := map[string]interface{}{
		"id": s.node,
		"stats": &nodeStats{
			Active:   true,
			Mining:   mining,
			Hashrate: hashrate,
			Peers:    s.server.PeerCount(),
			GasPrice: gasprice,
			Syncing:  syncing,
			Uptime:   100,
		},
	}
	report := map[string][]interface{}{
		"emit": {"stats", stats},
	}
	return conn.WriteJSON(report)
}
