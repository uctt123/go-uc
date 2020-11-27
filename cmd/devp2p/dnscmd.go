















package main

import (
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/console/prompt"
	"github.com/ethereum/go-ethereum/p2p/dnsdisc"
	"github.com/ethereum/go-ethereum/p2p/enode"
	cli "gopkg.in/urfave/cli.v1"
)

var (
	dnsCommand = cli.Command{
		Name:  "dns",
		Usage: "DNS Discovery Commands",
		Subcommands: []cli.Command{
			dnsSyncCommand,
			dnsSignCommand,
			dnsTXTCommand,
			dnsCloudflareCommand,
			dnsRoute53Command,
		},
	}
	dnsSyncCommand = cli.Command{
		Name:      "sync",
		Usage:     "Download a DNS discovery tree",
		ArgsUsage: "<url> [ <directory> ]",
		Action:    dnsSync,
		Flags:     []cli.Flag{dnsTimeoutFlag},
	}
	dnsSignCommand = cli.Command{
		Name:      "sign",
		Usage:     "Sign a DNS discovery tree",
		ArgsUsage: "<tree-directory> <key-file>",
		Action:    dnsSign,
		Flags:     []cli.Flag{dnsDomainFlag, dnsSeqFlag},
	}
	dnsTXTCommand = cli.Command{
		Name:      "to-txt",
		Usage:     "Create a DNS TXT records for a discovery tree",
		ArgsUsage: "<tree-directory> <output-file>",
		Action:    dnsToTXT,
	}
	dnsCloudflareCommand = cli.Command{
		Name:      "to-cloudflare",
		Usage:     "Deploy DNS TXT records to CloudFlare",
		ArgsUsage: "<tree-directory>",
		Action:    dnsToCloudflare,
		Flags:     []cli.Flag{cloudflareTokenFlag, cloudflareZoneIDFlag},
	}
	dnsRoute53Command = cli.Command{
		Name:      "to-route53",
		Usage:     "Deploy DNS TXT records to Amazon Route53",
		ArgsUsage: "<tree-directory>",
		Action:    dnsToRoute53,
		Flags:     []cli.Flag{route53AccessKeyFlag, route53AccessSecretFlag, route53ZoneIDFlag},
	}
)

var (
	dnsTimeoutFlag = cli.DurationFlag{
		Name:  "timeout",
		Usage: "Timeout for DNS lookups",
	}
	dnsDomainFlag = cli.StringFlag{
		Name:  "domain",
		Usage: "Domain name of the tree",
	}
	dnsSeqFlag = cli.UintFlag{
		Name:  "seq",
		Usage: "New sequence number of the tree",
	}
)

const (
	rootTTL     = 30 * 60              
	treeNodeTTL = 4 * 7 * 24 * 60 * 60 
)


func dnsSync(ctx *cli.Context) error {
	var (
		c      = dnsClient(ctx)
		url    = ctx.Args().Get(0)
		outdir = ctx.Args().Get(1)
	)
	domain, _, err := dnsdisc.ParseURL(url)
	if err != nil {
		return err
	}
	if outdir == "" {
		outdir = domain
	}

	t, err := c.SyncTree(url)
	if err != nil {
		return err
	}
	def := treeToDefinition(url, t)
	def.Meta.LastModified = time.Now()
	writeTreeMetadata(outdir, def)
	writeTreeNodes(outdir, def)
	return nil
}

func dnsSign(ctx *cli.Context) error {
	if ctx.NArg() < 2 {
		return fmt.Errorf("need tree definition directory and key file as arguments")
	}
	var (
		defdir  = ctx.Args().Get(0)
		keyfile = ctx.Args().Get(1)
		def     = loadTreeDefinition(defdir)
		domain  = directoryName(defdir)
	)
	if def.Meta.URL != "" {
		d, _, err := dnsdisc.ParseURL(def.Meta.URL)
		if err != nil {
			return fmt.Errorf("invalid 'url' field: %v", err)
		}
		domain = d
	}
	if ctx.IsSet(dnsDomainFlag.Name) {
		domain = ctx.String(dnsDomainFlag.Name)
	}
	if ctx.IsSet(dnsSeqFlag.Name) {
		def.Meta.Seq = ctx.Uint(dnsSeqFlag.Name)
	} else {
		def.Meta.Seq++ 
	}
	t, err := dnsdisc.MakeTree(def.Meta.Seq, def.Nodes, def.Meta.Links)
	if err != nil {
		return err
	}

	key := loadSigningKey(keyfile)
	url, err := t.Sign(key, domain)
	if err != nil {
		return fmt.Errorf("can't sign: %v", err)
	}

	def = treeToDefinition(url, t)
	def.Meta.LastModified = time.Now()
	writeTreeMetadata(defdir, def)
	return nil
}

func directoryName(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		exit(err)
	}
	return filepath.Base(abs)
}


func dnsToTXT(ctx *cli.Context) error {
	if ctx.NArg() < 1 {
		return fmt.Errorf("need tree definition directory as argument")
	}
	output := ctx.Args().Get(1)
	if output == "" {
		output = "-" 
	}
	domain, t, err := loadTreeDefinitionForExport(ctx.Args().Get(0))
	if err != nil {
		return err
	}
	writeTXTJSON(output, t.ToTXT(domain))
	return nil
}


func dnsToCloudflare(ctx *cli.Context) error {
	if ctx.NArg() < 1 {
		return fmt.Errorf("need tree definition directory as argument")
	}
	domain, t, err := loadTreeDefinitionForExport(ctx.Args().Get(0))
	if err != nil {
		return err
	}
	client := newCloudflareClient(ctx)
	return client.deploy(domain, t)
}


func dnsToRoute53(ctx *cli.Context) error {
	if ctx.NArg() < 1 {
		return fmt.Errorf("need tree definition directory as argument")
	}
	domain, t, err := loadTreeDefinitionForExport(ctx.Args().Get(0))
	if err != nil {
		return err
	}
	client := newRoute53Client(ctx)
	return client.deploy(domain, t)
}


func loadSigningKey(keyfile string) *ecdsa.PrivateKey {
	keyjson, err := ioutil.ReadFile(keyfile)
	if err != nil {
		exit(fmt.Errorf("failed to read the keyfile at '%s': %v", keyfile, err))
	}
	password, _ := prompt.Stdin.PromptPassword("Please enter the password for '" + keyfile + "': ")
	key, err := keystore.DecryptKey(keyjson, password)
	if err != nil {
		exit(fmt.Errorf("error decrypting key: %v", err))
	}
	return key.PrivateKey
}


func dnsClient(ctx *cli.Context) *dnsdisc.Client {
	var cfg dnsdisc.Config
	if commandHasFlag(ctx, dnsTimeoutFlag) {
		cfg.Timeout = ctx.Duration(dnsTimeoutFlag.Name)
	}
	return dnsdisc.NewClient(cfg)
}















type dnsDefinition struct {
	Meta  dnsMetaJSON
	Nodes []*enode.Node
}

type dnsMetaJSON struct {
	URL          string    `json:"url,omitempty"`
	Seq          uint      `json:"seq"`
	Sig          string    `json:"signature,omitempty"`
	Links        []string  `json:"links"`
	LastModified time.Time `json:"lastModified"`
}

func treeToDefinition(url string, t *dnsdisc.Tree) *dnsDefinition {
	meta := dnsMetaJSON{
		URL:   url,
		Seq:   t.Seq(),
		Sig:   t.Signature(),
		Links: t.Links(),
	}
	if meta.Links == nil {
		meta.Links = []string{}
	}
	return &dnsDefinition{Meta: meta, Nodes: t.Nodes()}
}


func loadTreeDefinition(directory string) *dnsDefinition {
	metaFile, nodesFile := treeDefinitionFiles(directory)
	var def dnsDefinition
	err := common.LoadJSON(metaFile, &def.Meta)
	if err != nil && !os.IsNotExist(err) {
		exit(err)
	}
	if def.Meta.Links == nil {
		def.Meta.Links = []string{}
	}
	
	for _, link := range def.Meta.Links {
		if _, _, err := dnsdisc.ParseURL(link); err != nil {
			exit(fmt.Errorf("invalid link %q: %v", link, err))
		}
	}
	
	nodes := loadNodesJSON(nodesFile)
	if err := nodes.verify(); err != nil {
		exit(err)
	}
	def.Nodes = nodes.nodes()
	return &def
}


func loadTreeDefinitionForExport(dir string) (domain string, t *dnsdisc.Tree, err error) {
	metaFile, _ := treeDefinitionFiles(dir)
	def := loadTreeDefinition(dir)
	if def.Meta.URL == "" {
		return "", nil, fmt.Errorf("missing 'url' field in %v", metaFile)
	}
	domain, pubkey, err := dnsdisc.ParseURL(def.Meta.URL)
	if err != nil {
		return "", nil, fmt.Errorf("invalid 'url' field in %v: %v", metaFile, err)
	}
	if t, err = dnsdisc.MakeTree(def.Meta.Seq, def.Nodes, def.Meta.Links); err != nil {
		return "", nil, err
	}
	if err := ensureValidTreeSignature(t, pubkey, def.Meta.Sig); err != nil {
		return "", nil, err
	}
	return domain, t, nil
}



func ensureValidTreeSignature(t *dnsdisc.Tree, pubkey *ecdsa.PublicKey, sig string) error {
	if sig == "" {
		return fmt.Errorf("missing signature, run 'devp2p dns sign' first")
	}
	if err := t.SetSignature(pubkey, sig); err != nil {
		return fmt.Errorf("invalid signature on tree, run 'devp2p dns sign' to update it")
	}
	return nil
}


func writeTreeMetadata(directory string, def *dnsDefinition) {
	metaJSON, err := json.MarshalIndent(&def.Meta, "", jsonIndent)
	if err != nil {
		exit(err)
	}
	if err := os.Mkdir(directory, 0744); err != nil && !os.IsExist(err) {
		exit(err)
	}
	metaFile, _ := treeDefinitionFiles(directory)
	if err := ioutil.WriteFile(metaFile, metaJSON, 0644); err != nil {
		exit(err)
	}
}

func writeTreeNodes(directory string, def *dnsDefinition) {
	ns := make(nodeSet, len(def.Nodes))
	ns.add(def.Nodes...)
	_, nodesFile := treeDefinitionFiles(directory)
	writeNodesJSON(nodesFile, ns)
}

func treeDefinitionFiles(directory string) (string, string) {
	meta := filepath.Join(directory, "enrtree-info.json")
	nodes := filepath.Join(directory, "nodes.json")
	return meta, nodes
}


func writeTXTJSON(file string, txt map[string]string) {
	txtJSON, err := json.MarshalIndent(txt, "", jsonIndent)
	if err != nil {
		exit(err)
	}
	if file == "-" {
		os.Stdout.Write(txtJSON)
		fmt.Println()
		return
	}
	if err := ioutil.WriteFile(file, txtJSON, 0644); err != nil {
		exit(err)
	}
}
