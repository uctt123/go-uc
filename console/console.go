















package console

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"

	"github.com/dop251/goja"
	"github.com/ethereum/go-ethereum/console/prompt"
	"github.com/ethereum/go-ethereum/internal/jsre"
	"github.com/ethereum/go-ethereum/internal/jsre/deps"
	"github.com/ethereum/go-ethereum/internal/web3ext"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/mattn/go-colorable"
	"github.com/peterh/liner"
)

var (
	
	passwordRegexp = regexp.MustCompile(`personal.[nusi]`)
	onlyWhitespace = regexp.MustCompile(`^\s*$`)
	exit           = regexp.MustCompile(`^\s*exit\s*;*\s*$`)
)


const HistoryFile = "history"


const DefaultPrompt = "> "



type Config struct {
	DataDir  string              
	DocRoot  string              
	Client   *rpc.Client         
	Prompt   string              
	Prompter prompt.UserPrompter 
	Printer  io.Writer           
	Preload  []string            
}




type Console struct {
	client   *rpc.Client         
	jsre     *jsre.JSRE          
	prompt   string              
	prompter prompt.UserPrompter 
	histPath string              
	history  []string            
	printer  io.Writer           
}



func New(config Config) (*Console, error) {
	
	if config.Prompter == nil {
		config.Prompter = prompt.Stdin
	}
	if config.Prompt == "" {
		config.Prompt = DefaultPrompt
	}
	if config.Printer == nil {
		config.Printer = colorable.NewColorableStdout()
	}

	
	console := &Console{
		client:   config.Client,
		jsre:     jsre.New(config.DocRoot, config.Printer),
		prompt:   config.Prompt,
		prompter: config.Prompter,
		printer:  config.Printer,
		histPath: filepath.Join(config.DataDir, HistoryFile),
	}
	if err := os.MkdirAll(config.DataDir, 0700); err != nil {
		return nil, err
	}
	if err := console.init(config.Preload); err != nil {
		return nil, err
	}
	return console, nil
}



func (c *Console) init(preload []string) error {
	c.initConsoleObject()

	
	bridge := newBridge(c.client, c.prompter, c.printer)
	if err := c.initWeb3(bridge); err != nil {
		return err
	}
	if err := c.initExtensions(); err != nil {
		return err
	}

	
	c.jsre.Do(func(vm *goja.Runtime) {
		c.initAdmin(vm, bridge)
		c.initPersonal(vm, bridge)
	})

	
	for _, path := range preload {
		if err := c.jsre.Exec(path); err != nil {
			failure := err.Error()
			if gojaErr, ok := err.(*goja.Exception); ok {
				failure = gojaErr.String()
			}
			return fmt.Errorf("%s: %v", path, failure)
		}
	}

	
	if c.prompter != nil {
		if content, err := ioutil.ReadFile(c.histPath); err != nil {
			c.prompter.SetHistory(nil)
		} else {
			c.history = strings.Split(string(content), "\n")
			c.prompter.SetHistory(c.history)
		}
		c.prompter.SetWordCompleter(c.AutoCompleteInput)
	}
	return nil
}

func (c *Console) initConsoleObject() {
	c.jsre.Do(func(vm *goja.Runtime) {
		console := vm.NewObject()
		console.Set("log", c.consoleOutput)
		console.Set("error", c.consoleOutput)
		vm.Set("console", console)
	})
}

func (c *Console) initWeb3(bridge *bridge) error {
	bnJS := string(deps.MustAsset("bignumber.js"))
	web3JS := string(deps.MustAsset("web3.js"))
	if err := c.jsre.Compile("bignumber.js", bnJS); err != nil {
		return fmt.Errorf("bignumber.js: %v", err)
	}
	if err := c.jsre.Compile("web3.js", web3JS); err != nil {
		return fmt.Errorf("web3.js: %v", err)
	}
	if _, err := c.jsre.Run("var Web3 = require('web3');"); err != nil {
		return fmt.Errorf("web3 require: %v", err)
	}
	var err error
	c.jsre.Do(func(vm *goja.Runtime) {
		transport := vm.NewObject()
		transport.Set("send", jsre.MakeCallback(vm, bridge.Send))
		transport.Set("sendAsync", jsre.MakeCallback(vm, bridge.Send))
		vm.Set("_consoleWeb3Transport", transport)
		_, err = vm.RunString("var web3 = new Web3(_consoleWeb3Transport)")
	})
	return err
}


func (c *Console) initExtensions() error {
	
	apis, err := c.client.SupportedModules()
	if err != nil {
		return fmt.Errorf("api modules: %v", err)
	}
	aliases := map[string]struct{}{"eth": {}, "personal": {}}
	for api := range apis {
		if api == "web3" {
			continue
		}
		aliases[api] = struct{}{}
		if file, ok := web3ext.Modules[api]; ok {
			if err = c.jsre.Compile(api+".js", file); err != nil {
				return fmt.Errorf("%s.js: %v", api, err)
			}
		}
	}

	
	c.jsre.Do(func(vm *goja.Runtime) {
		web3 := getObject(vm, "web3")
		for name := range aliases {
			if v := web3.Get(name); v != nil {
				vm.Set(name, v)
			}
		}
	})
	return nil
}


func (c *Console) initAdmin(vm *goja.Runtime, bridge *bridge) {
	if admin := getObject(vm, "admin"); admin != nil {
		admin.Set("sleepBlocks", jsre.MakeCallback(vm, bridge.SleepBlocks))
		admin.Set("sleep", jsre.MakeCallback(vm, bridge.Sleep))
		admin.Set("clearHistory", c.clearHistory)
	}
}







func (c *Console) initPersonal(vm *goja.Runtime, bridge *bridge) {
	personal := getObject(vm, "personal")
	if personal == nil || c.prompter == nil {
		return
	}
	jeth := vm.NewObject()
	vm.Set("jeth", jeth)
	jeth.Set("openWallet", personal.Get("openWallet"))
	jeth.Set("unlockAccount", personal.Get("unlockAccount"))
	jeth.Set("newAccount", personal.Get("newAccount"))
	jeth.Set("sign", personal.Get("sign"))
	personal.Set("openWallet", jsre.MakeCallback(vm, bridge.OpenWallet))
	personal.Set("unlockAccount", jsre.MakeCallback(vm, bridge.UnlockAccount))
	personal.Set("newAccount", jsre.MakeCallback(vm, bridge.NewAccount))
	personal.Set("sign", jsre.MakeCallback(vm, bridge.Sign))
}

func (c *Console) clearHistory() {
	c.history = nil
	c.prompter.ClearHistory()
	if err := os.Remove(c.histPath); err != nil {
		fmt.Fprintln(c.printer, "can't delete history file:", err)
	} else {
		fmt.Fprintln(c.printer, "history file deleted")
	}
}



func (c *Console) consoleOutput(call goja.FunctionCall) goja.Value {
	var output []string
	for _, argument := range call.Arguments {
		output = append(output, fmt.Sprintf("%v", argument))
	}
	fmt.Fprintln(c.printer, strings.Join(output, " "))
	return goja.Null()
}



func (c *Console) AutoCompleteInput(line string, pos int) (string, []string, string) {
	
	if len(line) == 0 || pos == 0 {
		return "", nil, ""
	}
	
	
	start := pos - 1
	for ; start > 0; start-- {
		
		if line[start] == '.' || (line[start] >= 'a' && line[start] <= 'z') || (line[start] >= 'A' && line[start] <= 'Z') {
			continue
		}
		
		if start >= 3 && line[start-3:start] == "web3" {
			start -= 3
			continue
		}
		
		start++
		break
	}
	return line[:start], c.jsre.CompleteKeywords(line[start:pos]), line[pos:]
}



func (c *Console) Welcome() {
	message := "Welcome to the Geth JavaScript console!\n\n"

	
	if res, err := c.jsre.Run(`
		var message = "instance: " + web3.version.node + "\n";
		try {
			message += "coinbase: " + eth.coinbase + "\n";
		} catch (err) {}
		message += "at block: " + eth.blockNumber + " (" + new Date(1000 * eth.getBlock(eth.blockNumber).timestamp) + ")\n";
		try {
			message += " datadir: " + admin.datadir + "\n";
		} catch (err) {}
		message
	`); err == nil {
		message += res.String()
	}
	
	if apis, err := c.client.SupportedModules(); err == nil {
		modules := make([]string, 0, len(apis))
		for api, version := range apis {
			modules = append(modules, fmt.Sprintf("%s:%s", api, version))
		}
		sort.Strings(modules)
		message += " modules: " + strings.Join(modules, " ") + "\n"
	}
	fmt.Fprintln(c.printer, message)
}



func (c *Console) Evaluate(statement string) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(c.printer, "[native] error: %v\n", r)
		}
	}()
	c.jsre.Evaluate(statement, c.printer)
}



func (c *Console) Interactive() {
	var (
		prompt      = c.prompt             
		indents     = 0                    
		input       = ""                   
		inputLine   = make(chan string, 1) 
		inputErr    = make(chan error, 1)  
		requestLine = make(chan string)    
		interrupt   = make(chan os.Signal, 1)
	)

	
	
	
	
	signal.Notify(interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(interrupt)

	
	go c.readLines(inputLine, inputErr, requestLine)
	defer close(requestLine)

	for {
		
		requestLine <- prompt

		select {
		case <-interrupt:
			fmt.Fprintln(c.printer, "caught interrupt, exiting")
			return

		case err := <-inputErr:
			if err == liner.ErrPromptAborted && indents > 0 {
				
				
				prompt, indents, input = c.prompt, 0, ""
				continue
			}
			return

		case line := <-inputLine:
			
			if indents <= 0 && exit.MatchString(line) {
				return
			}
			if onlyWhitespace.MatchString(line) {
				continue
			}
			
			input += line + "\n"
			indents = countIndents(input)
			if indents <= 0 {
				prompt = c.prompt
			} else {
				prompt = strings.Repeat(".", indents*3) + " "
			}
			
			if indents <= 0 {
				if len(input) > 0 && input[0] != ' ' && !passwordRegexp.MatchString(input) {
					if command := strings.TrimSpace(input); len(c.history) == 0 || command != c.history[len(c.history)-1] {
						c.history = append(c.history, command)
						if c.prompter != nil {
							c.prompter.AppendHistory(command)
						}
					}
				}
				c.Evaluate(input)
				input = ""
			}
		}
	}
}


func (c *Console) readLines(input chan<- string, errc chan<- error, prompt <-chan string) {
	for p := range prompt {
		line, err := c.prompter.PromptInput(p)
		if err != nil {
			errc <- err
		} else {
			input <- line
		}
	}
}



func countIndents(input string) int {
	var (
		indents     = 0
		inString    = false
		strOpenChar = ' '   
		charEscaped = false 
	)

	for _, c := range input {
		switch c {
		case '\\':
			
			if !charEscaped && inString {
				charEscaped = true
			}
		case '\'', '"':
			if inString && !charEscaped && strOpenChar == c { 
				inString = false
			} else if !inString && !charEscaped { 
				inString = true
				strOpenChar = c
			}
			charEscaped = false
		case '{', '(':
			if !inString { 
				indents++
			}
			charEscaped = false
		case '}', ')':
			if !inString {
				indents--
			}
			charEscaped = false
		default:
			charEscaped = false
		}
	}

	return indents
}


func (c *Console) Execute(path string) error {
	return c.jsre.Exec(path)
}


func (c *Console) Stop(graceful bool) error {
	if err := ioutil.WriteFile(c.histPath, []byte(strings.Join(c.history, "\n")), 0600); err != nil {
		return err
	}
	if err := os.Chmod(c.histPath, 0600); err != nil { 
		return err
	}
	c.jsre.Stop(graceful)
	return nil
}
