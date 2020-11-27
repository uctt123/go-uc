















package les

import (
	"errors"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/mclock"
	lps "github.com/ethereum/go-ethereum/les/lespay/server"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

var (
	errNoCheckpoint         = errors.New("no local checkpoint provided")
	errNotActivated         = errors.New("checkpoint registrar is not activated")
	errUnknownBenchmarkType = errors.New("unknown benchmark type")
	errNoPriority           = errors.New("priority too low to raise capacity")
)


type PrivateLightServerAPI struct {
	server                               *LesServer
	defaultPosFactors, defaultNegFactors lps.PriceFactors
}


func NewPrivateLightServerAPI(server *LesServer) *PrivateLightServerAPI {
	return &PrivateLightServerAPI{
		server:            server,
		defaultPosFactors: server.clientPool.defaultPosFactors,
		defaultNegFactors: server.clientPool.defaultNegFactors,
	}
}


func (api *PrivateLightServerAPI) ServerInfo() map[string]interface{} {
	res := make(map[string]interface{})
	res["minimumCapacity"] = api.server.minCapacity
	res["maximumCapacity"] = api.server.maxCapacity
	res["totalCapacity"], res["totalConnectedCapacity"], res["priorityConnectedCapacity"] = api.server.clientPool.capacityInfo()
	return res
}


func (api *PrivateLightServerAPI) ClientInfo(ids []enode.ID) map[enode.ID]map[string]interface{} {
	res := make(map[enode.ID]map[string]interface{})
	api.server.clientPool.forClients(ids, func(client *clientInfo) {
		res[client.node.ID()] = api.clientInfo(client)
	})
	return res
}







func (api *PrivateLightServerAPI) PriorityClientInfo(start, stop enode.ID, maxCount int) map[enode.ID]map[string]interface{} {
	res := make(map[enode.ID]map[string]interface{})
	ids := api.server.clientPool.bt.GetPosBalanceIDs(start, stop, maxCount+1)
	if len(ids) > maxCount {
		res[ids[maxCount]] = make(map[string]interface{})
		ids = ids[:maxCount]
	}
	if len(ids) != 0 {
		api.server.clientPool.forClients(ids, func(client *clientInfo) {
			res[client.node.ID()] = api.clientInfo(client)
		})
	}
	return res
}


func (api *PrivateLightServerAPI) clientInfo(c *clientInfo) map[string]interface{} {
	info := make(map[string]interface{})
	pb, nb := c.balance.GetBalance()
	info["isConnected"] = c.connected
	info["pricing/balance"] = pb
	info["priority"] = pb != 0
	
	
	if c.connected {
		info["connectionTime"] = float64(mclock.Now()-c.connectedAt) / float64(time.Second)
		info["capacity"], _ = api.server.clientPool.ns.GetField(c.node, priorityPoolSetup.CapacityField).(uint64)
		info["pricing/negBalance"] = nb
	}
	return info
}



func (api *PrivateLightServerAPI) setParams(params map[string]interface{}, client *clientInfo, posFactors, negFactors *lps.PriceFactors) (updateFactors bool, err error) {
	defParams := client == nil
	for name, value := range params {
		errValue := func() error {
			return fmt.Errorf("invalid value for parameter '%s'", name)
		}
		setFactor := func(v *float64) {
			if val, ok := value.(float64); ok && val >= 0 {
				*v = val / float64(time.Second)
				updateFactors = true
			} else {
				err = errValue()
			}
		}

		switch {
		case name == "pricing/timeFactor":
			setFactor(&posFactors.TimeFactor)
		case name == "pricing/capacityFactor":
			setFactor(&posFactors.CapacityFactor)
		case name == "pricing/requestCostFactor":
			setFactor(&posFactors.RequestFactor)
		case name == "pricing/negative/timeFactor":
			setFactor(&negFactors.TimeFactor)
		case name == "pricing/negative/capacityFactor":
			setFactor(&negFactors.CapacityFactor)
		case name == "pricing/negative/requestCostFactor":
			setFactor(&negFactors.RequestFactor)
		case !defParams && name == "capacity":
			if capacity, ok := value.(float64); ok && uint64(capacity) >= api.server.minCapacity {
				_, err = api.server.clientPool.setCapacity(client.node, client.address, uint64(capacity), 0, true)
				
				
			} else {
				err = errValue()
			}
		default:
			if defParams {
				err = fmt.Errorf("invalid default parameter '%s'", name)
			} else {
				err = fmt.Errorf("invalid client parameter '%s'", name)
			}
		}
		if err != nil {
			return
		}
	}
	return
}



func (api *PrivateLightServerAPI) SetClientParams(ids []enode.ID, params map[string]interface{}) error {
	var err error
	api.server.clientPool.forClients(ids, func(client *clientInfo) {
		if client.connected {
			posFactors, negFactors := client.balance.GetPriceFactors()
			update, e := api.setParams(params, client, &posFactors, &negFactors)
			if update {
				client.balance.SetPriceFactors(posFactors, negFactors)
			}
			if e != nil {
				err = e
			}
		} else {
			err = fmt.Errorf("client %064x is not connected", client.node.ID())
		}
	})
	return err
}


func (api *PrivateLightServerAPI) SetDefaultParams(params map[string]interface{}) error {
	update, err := api.setParams(params, nil, &api.defaultPosFactors, &api.defaultNegFactors)
	if update {
		api.server.clientPool.setDefaultFactors(api.defaultPosFactors, api.defaultNegFactors)
	}
	return err
}





func (api *PrivateLightServerAPI) SetConnectedBias(bias time.Duration) error {
	if bias < time.Duration(0) {
		return fmt.Errorf("bias illegal: %v less than 0", bias)
	}
	api.server.clientPool.setConnectedBias(bias)
	return nil
}



func (api *PrivateLightServerAPI) AddBalance(id enode.ID, amount int64) (balance [2]uint64, err error) {
	api.server.clientPool.forClients([]enode.ID{id}, func(c *clientInfo) {
		balance[0], balance[1], err = c.balance.AddBalance(amount)
	})
	return
}







func (api *PrivateLightServerAPI) Benchmark(setups []map[string]interface{}, passCount, length int) ([]map[string]interface{}, error) {
	benchmarks := make([]requestBenchmark, len(setups))
	for i, setup := range setups {
		if t, ok := setup["type"].(string); ok {
			getInt := func(field string, def int) int {
				if value, ok := setup[field].(float64); ok {
					return int(value)
				}
				return def
			}
			getBool := func(field string, def bool) bool {
				if value, ok := setup[field].(bool); ok {
					return value
				}
				return def
			}
			switch t {
			case "header":
				benchmarks[i] = &benchmarkBlockHeaders{
					amount:  getInt("amount", 1),
					skip:    getInt("skip", 1),
					byHash:  getBool("byHash", false),
					reverse: getBool("reverse", false),
				}
			case "body":
				benchmarks[i] = &benchmarkBodiesOrReceipts{receipts: false}
			case "receipts":
				benchmarks[i] = &benchmarkBodiesOrReceipts{receipts: true}
			case "proof":
				benchmarks[i] = &benchmarkProofsOrCode{code: false}
			case "code":
				benchmarks[i] = &benchmarkProofsOrCode{code: true}
			case "cht":
				benchmarks[i] = &benchmarkHelperTrie{
					bloom:    false,
					reqCount: getInt("amount", 1),
				}
			case "bloom":
				benchmarks[i] = &benchmarkHelperTrie{
					bloom:    true,
					reqCount: getInt("amount", 1),
				}
			case "txSend":
				benchmarks[i] = &benchmarkTxSend{}
			case "txStatus":
				benchmarks[i] = &benchmarkTxStatus{}
			default:
				return nil, errUnknownBenchmarkType
			}
		} else {
			return nil, errUnknownBenchmarkType
		}
	}
	rs := api.server.handler.runBenchmark(benchmarks, passCount, time.Millisecond*time.Duration(length))
	result := make([]map[string]interface{}, len(setups))
	for i, r := range rs {
		res := make(map[string]interface{})
		if r.err == nil {
			res["totalCount"] = r.totalCount
			res["avgTime"] = r.avgTime
			res["maxInSize"] = r.maxInSize
			res["maxOutSize"] = r.maxOutSize
		} else {
			res["error"] = r.err.Error()
		}
		result[i] = res
	}
	return result, nil
}


type PrivateDebugAPI struct {
	server *LesServer
}


func NewPrivateDebugAPI(server *LesServer) *PrivateDebugAPI {
	return &PrivateDebugAPI{
		server: server,
	}
}


func (api *PrivateDebugAPI) FreezeClient(id enode.ID) error {
	var err error
	api.server.clientPool.forClients([]enode.ID{id}, func(c *clientInfo) {
		if c.connected {
			c.peer.freeze()
		} else {
			err = fmt.Errorf("client %064x is not connected", id[:])
		}
	})
	return err
}


type PrivateLightAPI struct {
	backend *lesCommons
}


func NewPrivateLightAPI(backend *lesCommons) *PrivateLightAPI {
	return &PrivateLightAPI{backend: backend}
}








func (api *PrivateLightAPI) LatestCheckpoint() ([4]string, error) {
	var res [4]string
	cp := api.backend.latestLocalCheckpoint()
	if cp.Empty() {
		return res, errNoCheckpoint
	}
	res[0] = hexutil.EncodeUint64(cp.SectionIndex)
	res[1], res[2], res[3] = cp.SectionHead.Hex(), cp.CHTRoot.Hex(), cp.BloomRoot.Hex()
	return res, nil
}







func (api *PrivateLightAPI) GetCheckpoint(index uint64) ([3]string, error) {
	var res [3]string
	cp := api.backend.localCheckpoint(index)
	if cp.Empty() {
		return res, errNoCheckpoint
	}
	res[0], res[1], res[2] = cp.SectionHead.Hex(), cp.CHTRoot.Hex(), cp.BloomRoot.Hex()
	return res, nil
}


func (api *PrivateLightAPI) GetCheckpointContractAddress() (string, error) {
	if api.backend.oracle == nil {
		return "", errNotActivated
	}
	return api.backend.oracle.Contract().ContractAddr().Hex(), nil
}
