















package console

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/ethereum/go-ethereum/accounts/scwallet"
	"github.com/ethereum/go-ethereum/accounts/usbwallet"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/console/prompt"
	"github.com/ethereum/go-ethereum/internal/jsre"
	"github.com/ethereum/go-ethereum/rpc"
)



type bridge struct {
	client   *rpc.Client         
	prompter prompt.UserPrompter 
	printer  io.Writer           
}


func newBridge(client *rpc.Client, prompter prompt.UserPrompter, printer io.Writer) *bridge {
	return &bridge{
		client:   client,
		prompter: prompter,
		printer:  printer,
	}
}

func getJeth(vm *goja.Runtime) *goja.Object {
	jeth := vm.Get("jeth")
	if jeth == nil {
		panic(vm.ToValue("jeth object does not exist"))
	}
	return jeth.ToObject(vm)
}




func (b *bridge) NewAccount(call jsre.Call) (goja.Value, error) {
	var (
		password string
		confirm  string
		err      error
	)
	switch {
	
	case len(call.Arguments) == 0:
		if password, err = b.prompter.PromptPassword("Passphrase: "); err != nil {
			return nil, err
		}
		if confirm, err = b.prompter.PromptPassword("Repeat passphrase: "); err != nil {
			return nil, err
		}
		if password != confirm {
			return nil, fmt.Errorf("passwords don't match!")
		}
	
	case len(call.Arguments) == 1 && call.Argument(0).ToString() != nil:
		password = call.Argument(0).ToString().String()
	default:
		return nil, fmt.Errorf("expected 0 or 1 string argument")
	}
	
	newAccount, callable := goja.AssertFunction(getJeth(call.VM).Get("newAccount"))
	if !callable {
		return nil, fmt.Errorf("jeth.newAccount is not callable")
	}
	ret, err := newAccount(goja.Null(), call.VM.ToValue(password))
	if err != nil {
		return nil, err
	}
	return ret, nil
}



func (b *bridge) OpenWallet(call jsre.Call) (goja.Value, error) {
	
	if call.Argument(0).ToObject(call.VM).ClassName() != "String" {
		return nil, fmt.Errorf("first argument must be the wallet URL to open")
	}
	wallet := call.Argument(0)

	var passwd goja.Value
	if goja.IsUndefined(call.Argument(1)) || goja.IsNull(call.Argument(1)) {
		passwd = call.VM.ToValue("")
	} else {
		passwd = call.Argument(1)
	}
	
	openWallet, callable := goja.AssertFunction(getJeth(call.VM).Get("openWallet"))
	if !callable {
		return nil, fmt.Errorf("jeth.openWallet is not callable")
	}
	val, err := openWallet(goja.Null(), wallet, passwd)
	if err == nil {
		return val, nil
	}

	
	switch {
	case strings.HasSuffix(err.Error(), usbwallet.ErrTrezorPINNeeded.Error()):
		val, err = b.readPinAndReopenWallet(call)
		if err == nil {
			return val, nil
		}
		val, err = b.readPassphraseAndReopenWallet(call)
		if err != nil {
			return nil, err
		}

	case strings.HasSuffix(err.Error(), scwallet.ErrPairingPasswordNeeded.Error()):
		
		input, err := b.prompter.PromptPassword("Please enter the pairing password: ")
		if err != nil {
			return nil, err
		}
		passwd = call.VM.ToValue(input)
		if val, err = openWallet(goja.Null(), wallet, passwd); err != nil {
			if !strings.HasSuffix(err.Error(), scwallet.ErrPINNeeded.Error()) {
				return nil, err
			} else {
				
				input, err := b.prompter.PromptPassword("Please enter current PIN: ")
				if err != nil {
					return nil, err
				}
				if val, err = openWallet(goja.Null(), wallet, call.VM.ToValue(input)); err != nil {
					return nil, err
				}
			}
		}

	case strings.HasSuffix(err.Error(), scwallet.ErrPINUnblockNeeded.Error()):
		
		var pukpin string
		input, err := b.prompter.PromptPassword("Please enter current PUK: ")
		if err != nil {
			return nil, err
		}
		pukpin = input
		input, err = b.prompter.PromptPassword("Please enter new PIN: ")
		if err != nil {
			return nil, err
		}
		pukpin += input

		if val, err = openWallet(goja.Null(), wallet, call.VM.ToValue(pukpin)); err != nil {
			return nil, err
		}

	case strings.HasSuffix(err.Error(), scwallet.ErrPINNeeded.Error()):
		
		input, err := b.prompter.PromptPassword("Please enter current PIN: ")
		if err != nil {
			return nil, err
		}
		if val, err = openWallet(goja.Null(), wallet, call.VM.ToValue(input)); err != nil {
			return nil, err
		}

	default:
		
		return nil, err
	}
	return val, nil
}

func (b *bridge) readPassphraseAndReopenWallet(call jsre.Call) (goja.Value, error) {
	wallet := call.Argument(0)
	input, err := b.prompter.PromptPassword("Please enter your passphrase: ")
	if err != nil {
		return nil, err
	}
	openWallet, callable := goja.AssertFunction(getJeth(call.VM).Get("openWallet"))
	if !callable {
		return nil, fmt.Errorf("jeth.openWallet is not callable")
	}
	return openWallet(goja.Null(), wallet, call.VM.ToValue(input))
}

func (b *bridge) readPinAndReopenWallet(call jsre.Call) (goja.Value, error) {
	wallet := call.Argument(0)
	
	fmt.Fprintf(b.printer, "Look at the device for number positions\n\n")
	fmt.Fprintf(b.printer, "7 | 8 | 9\n")
	fmt.Fprintf(b.printer, "--+---+--\n")
	fmt.Fprintf(b.printer, "4 | 5 | 6\n")
	fmt.Fprintf(b.printer, "--+---+--\n")
	fmt.Fprintf(b.printer, "1 | 2 | 3\n\n")

	input, err := b.prompter.PromptPassword("Please enter current PIN: ")
	if err != nil {
		return nil, err
	}
	openWallet, callable := goja.AssertFunction(getJeth(call.VM).Get("openWallet"))
	if !callable {
		return nil, fmt.Errorf("jeth.openWallet is not callable")
	}
	return openWallet(goja.Null(), wallet, call.VM.ToValue(input))
}





func (b *bridge) UnlockAccount(call jsre.Call) (goja.Value, error) {
	if len(call.Arguments) < 1 {
		return nil, fmt.Errorf("usage: unlockAccount(account, [ password, duration ])")
	}

	account := call.Argument(0)
	
	if goja.IsUndefined(account) || goja.IsNull(account) || account.ExportType().Kind() != reflect.String {
		return nil, fmt.Errorf("first argument must be the account to unlock")
	}

	
	var passwd goja.Value
	if goja.IsUndefined(call.Argument(1)) || goja.IsNull(call.Argument(1)) {
		fmt.Fprintf(b.printer, "Unlock account %s\n", account)
		input, err := b.prompter.PromptPassword("Passphrase: ")
		if err != nil {
			return nil, err
		}
		passwd = call.VM.ToValue(input)
	} else {
		if call.Argument(1).ExportType().Kind() != reflect.String {
			return nil, fmt.Errorf("password must be a string")
		}
		passwd = call.Argument(1)
	}

	
	duration := goja.Null()
	if !goja.IsUndefined(call.Argument(2)) && !goja.IsNull(call.Argument(2)) {
		if !isNumber(call.Argument(2)) {
			return nil, fmt.Errorf("unlock duration must be a number")
		}
		duration = call.Argument(2)
	}

	
	unlockAccount, callable := goja.AssertFunction(getJeth(call.VM).Get("unlockAccount"))
	if !callable {
		return nil, fmt.Errorf("jeth.unlockAccount is not callable")
	}
	return unlockAccount(goja.Null(), account, passwd, duration)
}




func (b *bridge) Sign(call jsre.Call) (goja.Value, error) {
	if nArgs := len(call.Arguments); nArgs < 2 {
		return nil, fmt.Errorf("usage: sign(message, account, [ password ])")
	}
	var (
		message = call.Argument(0)
		account = call.Argument(1)
		passwd  = call.Argument(2)
	)

	if goja.IsUndefined(message) || message.ExportType().Kind() != reflect.String {
		return nil, fmt.Errorf("first argument must be the message to sign")
	}
	if goja.IsUndefined(account) || account.ExportType().Kind() != reflect.String {
		return nil, fmt.Errorf("second argument must be the account to sign with")
	}

	
	if goja.IsUndefined(passwd) || goja.IsNull(passwd) {
		fmt.Fprintf(b.printer, "Give password for account %s\n", account)
		input, err := b.prompter.PromptPassword("Password: ")
		if err != nil {
			return nil, err
		}
		passwd = call.VM.ToValue(input)
	} else if passwd.ExportType().Kind() != reflect.String {
		return nil, fmt.Errorf("third argument must be the password to unlock the account")
	}

	
	sign, callable := goja.AssertFunction(getJeth(call.VM).Get("sign"))
	if !callable {
		return nil, fmt.Errorf("jeth.sign is not callable")
	}
	return sign(goja.Null(), message, account, passwd)
}


func (b *bridge) Sleep(call jsre.Call) (goja.Value, error) {
	if nArgs := len(call.Arguments); nArgs < 1 {
		return nil, fmt.Errorf("usage: sleep(<number of seconds>)")
	}
	sleepObj := call.Argument(0)
	if goja.IsUndefined(sleepObj) || goja.IsNull(sleepObj) || !isNumber(sleepObj) {
		return nil, fmt.Errorf("usage: sleep(<number of seconds>)")
	}
	sleep := sleepObj.ToFloat()
	time.Sleep(time.Duration(sleep * float64(time.Second)))
	return call.VM.ToValue(true), nil
}



func (b *bridge) SleepBlocks(call jsre.Call) (goja.Value, error) {
	
	var (
		blocks = int64(0)
		sleep  = int64(9999999999999999) 
	)
	nArgs := len(call.Arguments)
	if nArgs == 0 {
		return nil, fmt.Errorf("usage: sleepBlocks(<n blocks>[, max sleep in seconds])")
	}
	if nArgs >= 1 {
		if goja.IsNull(call.Argument(0)) || goja.IsUndefined(call.Argument(0)) || !isNumber(call.Argument(0)) {
			return nil, fmt.Errorf("expected number as first argument")
		}
		blocks = call.Argument(0).ToInteger()
	}
	if nArgs >= 2 {
		if goja.IsNull(call.Argument(1)) || goja.IsUndefined(call.Argument(1)) || !isNumber(call.Argument(1)) {
			return nil, fmt.Errorf("expected number as second argument")
		}
		sleep = call.Argument(1).ToInteger()
	}

	
	deadline := time.Now().Add(time.Duration(sleep) * time.Second)
	var lastNumber hexutil.Uint64
	if err := b.client.Call(&lastNumber, "eth_blockNumber"); err != nil {
		return nil, err
	}
	for time.Now().Before(deadline) {
		var number hexutil.Uint64
		if err := b.client.Call(&number, "eth_blockNumber"); err != nil {
			return nil, err
		}
		if number != lastNumber {
			lastNumber = number
			blocks--
		}
		if blocks <= 0 {
			break
		}
		time.Sleep(time.Second)
	}
	return call.VM.ToValue(true), nil
}

type jsonrpcCall struct {
	ID     int64
	Method string
	Params []interface{}
}


func (b *bridge) Send(call jsre.Call) (goja.Value, error) {
	
	reqVal, err := call.Argument(0).ToObject(call.VM).MarshalJSON()
	if err != nil {
		return nil, err
	}

	var (
		rawReq = string(reqVal)
		dec    = json.NewDecoder(strings.NewReader(rawReq))
		reqs   []jsonrpcCall
		batch  bool
	)
	dec.UseNumber() 
	if rawReq[0] == '[' {
		batch = true
		dec.Decode(&reqs)
	} else {
		batch = false
		reqs = make([]jsonrpcCall, 1)
		dec.Decode(&reqs[0])
	}

	
	var resps []*goja.Object
	for _, req := range reqs {
		resp := call.VM.NewObject()
		resp.Set("jsonrpc", "2.0")
		resp.Set("id", req.ID)

		var result json.RawMessage
		if err = b.client.Call(&result, req.Method, req.Params...); err == nil {
			if result == nil {
				
				
				resp.Set("result", goja.Null())
			} else {
				JSON := call.VM.Get("JSON").ToObject(call.VM)
				parse, callable := goja.AssertFunction(JSON.Get("parse"))
				if !callable {
					return nil, fmt.Errorf("JSON.parse is not a function")
				}
				resultVal, err := parse(goja.Null(), call.VM.ToValue(string(result)))
				if err != nil {
					setError(resp, -32603, err.Error(), nil)
				} else {
					resp.Set("result", resultVal)
				}
			}
		} else {
			code := -32603
			var data interface{}
			if err, ok := err.(rpc.Error); ok {
				code = err.ErrorCode()
			}
			if err, ok := err.(rpc.DataError); ok {
				data = err.ErrorData()
			}
			setError(resp, code, err.Error(), data)
		}
		resps = append(resps, resp)
	}
	
	
	var result goja.Value
	if batch {
		result = call.VM.ToValue(resps)
	} else {
		result = resps[0]
	}
	if fn, isFunc := goja.AssertFunction(call.Argument(1)); isFunc {
		fn(goja.Null(), goja.Null(), result)
		return goja.Undefined(), nil
	}
	return result, nil
}

func setError(resp *goja.Object, code int, msg string, data interface{}) {
	err := make(map[string]interface{})
	err["code"] = code
	err["message"] = msg
	if data != nil {
		err["data"] = data
	}
	resp.Set("error", err)
}


func isNumber(v goja.Value) bool {
	k := v.ExportType().Kind()
	return k >= reflect.Int && k <= reflect.Float64
}

func getObject(vm *goja.Runtime, name string) *goja.Object {
	v := vm.Get(name)
	if v == nil {
		return nil
	}
	return v.ToObject(vm)
}
