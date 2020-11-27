















package main

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/dnsdisc"
	"gopkg.in/urfave/cli.v1"
)

const (
	
	
	
	route53ChangeSizeLimit  = 32000
	route53ChangeCountLimit = 1000
)

var (
	route53AccessKeyFlag = cli.StringFlag{
		Name:   "access-key-id",
		Usage:  "AWS Access Key ID",
		EnvVar: "AWS_ACCESS_KEY_ID",
	}
	route53AccessSecretFlag = cli.StringFlag{
		Name:   "access-key-secret",
		Usage:  "AWS Access Key Secret",
		EnvVar: "AWS_SECRET_ACCESS_KEY",
	}
	route53ZoneIDFlag = cli.StringFlag{
		Name:  "zone-id",
		Usage: "Route53 Zone ID",
	}
)

type route53Client struct {
	api    *route53.Route53
	zoneID string
}

type recordSet struct {
	values []string
	ttl    int64
}


func newRoute53Client(ctx *cli.Context) *route53Client {
	akey := ctx.String(route53AccessKeyFlag.Name)
	asec := ctx.String(route53AccessSecretFlag.Name)
	if akey == "" || asec == "" {
		exit(fmt.Errorf("need Route53 Access Key ID and secret proceed"))
	}
	config := &aws.Config{Credentials: credentials.NewStaticCredentials(akey, asec, "")}
	session, err := session.NewSession(config)
	if err != nil {
		exit(fmt.Errorf("can't create AWS session: %v", err))
	}
	return &route53Client{
		api:    route53.New(session),
		zoneID: ctx.String(route53ZoneIDFlag.Name),
	}
}


func (c *route53Client) deploy(name string, t *dnsdisc.Tree) error {
	if err := c.checkZone(name); err != nil {
		return err
	}

	
	existing, err := c.collectRecords(name)
	if err != nil {
		return err
	}
	log.Info(fmt.Sprintf("Found %d TXT records", len(existing)))

	records := t.ToTXT(name)
	changes := c.computeChanges(name, records, existing)
	if len(changes) == 0 {
		log.Info("No DNS changes needed")
		return nil
	}

	
	batches := splitChanges(changes, route53ChangeSizeLimit, route53ChangeCountLimit)
	for i, changes := range batches {
		log.Info(fmt.Sprintf("Submitting %d changes to Route53", len(changes)))
		batch := new(route53.ChangeBatch)
		batch.SetChanges(changes)
		batch.SetComment(fmt.Sprintf("enrtree update %d/%d of %s at seq %d", i+1, len(batches), name, t.Seq()))
		req := &route53.ChangeResourceRecordSetsInput{HostedZoneId: &c.zoneID, ChangeBatch: batch}
		resp, err := c.api.ChangeResourceRecordSets(req)
		if err != nil {
			return err
		}

		log.Info(fmt.Sprintf("Waiting for change request %s", *resp.ChangeInfo.Id))
		wreq := &route53.GetChangeInput{Id: resp.ChangeInfo.Id}
		if err := c.api.WaitUntilResourceRecordSetsChanged(wreq); err != nil {
			return err
		}
	}
	return nil
}


func (c *route53Client) checkZone(name string) (err error) {
	if c.zoneID == "" {
		c.zoneID, err = c.findZoneID(name)
	}
	return err
}


func (c *route53Client) findZoneID(name string) (string, error) {
	log.Info(fmt.Sprintf("Finding Route53 Zone ID for %s", name))
	var req route53.ListHostedZonesByNameInput
	for {
		resp, err := c.api.ListHostedZonesByName(&req)
		if err != nil {
			return "", err
		}
		for _, zone := range resp.HostedZones {
			if isSubdomain(name, *zone.Name) {
				return *zone.Id, nil
			}
		}
		if !*resp.IsTruncated {
			break
		}
		req.DNSName = resp.NextDNSName
		req.HostedZoneId = resp.NextHostedZoneId
	}
	return "", errors.New("can't find zone ID for " + name)
}


func (c *route53Client) computeChanges(name string, records map[string]string, existing map[string]recordSet) []*route53.Change {
	
	lrecords := make(map[string]string, len(records))
	for name, r := range records {
		lrecords[strings.ToLower(name)] = r
	}
	records = lrecords

	var changes []*route53.Change
	for path, val := range records {
		ttl := int64(rootTTL)
		if path != name {
			ttl = int64(treeNodeTTL)
		}

		prevRecords, exists := existing[path]
		prevValue := strings.Join(prevRecords.values, "")
		if !exists {
			
			log.Info(fmt.Sprintf("Creating %s = %q", path, val))
			changes = append(changes, newTXTChange("CREATE", path, ttl, splitTXT(val)))
		} else if prevValue != val || prevRecords.ttl != ttl {
			
			log.Info(fmt.Sprintf("Updating %s from %q to %q", path, prevValue, val))
			changes = append(changes, newTXTChange("UPSERT", path, ttl, splitTXT(val)))
		} else {
			log.Info(fmt.Sprintf("Skipping %s = %q", path, val))
		}
	}

	
	for path, set := range existing {
		if _, ok := records[path]; ok {
			continue
		}
		
		log.Info(fmt.Sprintf("Deleting %s = %q", path, strings.Join(set.values, "")))
		changes = append(changes, newTXTChange("DELETE", path, set.ttl, set.values...))
	}

	sortChanges(changes)
	return changes
}


func sortChanges(changes []*route53.Change) {
	score := map[string]int{"CREATE": 1, "UPSERT": 2, "DELETE": 3}
	sort.Slice(changes, func(i, j int) bool {
		if *changes[i].Action == *changes[j].Action {
			return *changes[i].ResourceRecordSet.Name < *changes[j].ResourceRecordSet.Name
		}
		return score[*changes[i].Action] < score[*changes[j].Action]
	})
}



func splitChanges(changes []*route53.Change, sizeLimit, countLimit int) [][]*route53.Change {
	var (
		batches    [][]*route53.Change
		batchSize  int
		batchCount int
	)
	for _, ch := range changes {
		
		count := changeCount(ch)
		size := changeSize(ch) * count
		overSize := batchSize+size > sizeLimit
		overCount := batchCount+count > countLimit
		if len(batches) == 0 || overSize || overCount {
			batches = append(batches, nil)
			batchSize = 0
			batchCount = 0
		}
		batches[len(batches)-1] = append(batches[len(batches)-1], ch)
		batchSize += size
		batchCount += count
	}
	return batches
}


func changeSize(ch *route53.Change) int {
	size := 0
	for _, rr := range ch.ResourceRecordSet.ResourceRecords {
		if rr.Value != nil {
			size += len(*rr.Value)
		}
	}
	return size
}

func changeCount(ch *route53.Change) int {
	if *ch.Action == "UPSERT" {
		return 2
	}
	return 1
}


func (c *route53Client) collectRecords(name string) (map[string]recordSet, error) {
	log.Info(fmt.Sprintf("Retrieving existing TXT records on %s (%s)", name, c.zoneID))
	var req route53.ListResourceRecordSetsInput
	req.SetHostedZoneId(c.zoneID)
	existing := make(map[string]recordSet)
	err := c.api.ListResourceRecordSetsPages(&req, func(resp *route53.ListResourceRecordSetsOutput, last bool) bool {
		for _, set := range resp.ResourceRecordSets {
			if !isSubdomain(*set.Name, name) || *set.Type != "TXT" {
				continue
			}
			s := recordSet{ttl: *set.TTL}
			for _, rec := range set.ResourceRecords {
				s.values = append(s.values, *rec.Value)
			}
			name := strings.TrimSuffix(*set.Name, ".")
			existing[name] = s
		}
		return true
	})
	return existing, err
}


func newTXTChange(action, name string, ttl int64, values ...string) *route53.Change {
	var c route53.Change
	var r route53.ResourceRecordSet
	var rrs []*route53.ResourceRecord
	for _, val := range values {
		rr := new(route53.ResourceRecord)
		rr.SetValue(val)
		rrs = append(rrs, rr)
	}
	r.SetType("TXT")
	r.SetName(name)
	r.SetTTL(ttl)
	r.SetResourceRecords(rrs)
	c.SetAction(action)
	c.SetResourceRecordSet(&r)
	return &c
}


func isSubdomain(name, domain string) bool {
	domain = strings.TrimSuffix(domain, ".")
	name = strings.TrimSuffix(name, ".")
	return strings.HasSuffix("."+name, "."+domain)
}


func splitTXT(value string) string {
	var result strings.Builder
	for len(value) > 0 {
		rlen := len(value)
		if rlen > 253 {
			rlen = 253
		}
		result.WriteString(strconv.Quote(value[:rlen]))
		value = value[rlen:]
	}
	return result.String()
}
