















package main

import (
	"fmt"
	"strings"

	"github.com/cloudflare/cloudflare-go"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/dnsdisc"
	"gopkg.in/urfave/cli.v1"
)

var (
	cloudflareTokenFlag = cli.StringFlag{
		Name:   "token",
		Usage:  "CloudFlare API token",
		EnvVar: "CLOUDFLARE_API_TOKEN",
	}
	cloudflareZoneIDFlag = cli.StringFlag{
		Name:  "zoneid",
		Usage: "CloudFlare Zone ID (optional)",
	}
)

type cloudflareClient struct {
	*cloudflare.API
	zoneID string
}


func newCloudflareClient(ctx *cli.Context) *cloudflareClient {
	token := ctx.String(cloudflareTokenFlag.Name)
	if token == "" {
		exit(fmt.Errorf("need cloudflare API token to proceed"))
	}
	api, err := cloudflare.NewWithAPIToken(token)
	if err != nil {
		exit(fmt.Errorf("can't create Cloudflare client: %v", err))
	}
	return &cloudflareClient{
		API:    api,
		zoneID: ctx.String(cloudflareZoneIDFlag.Name),
	}
}


func (c *cloudflareClient) deploy(name string, t *dnsdisc.Tree) error {
	if err := c.checkZone(name); err != nil {
		return err
	}
	records := t.ToTXT(name)
	return c.uploadRecords(name, records)
}


func (c *cloudflareClient) checkZone(name string) error {
	if c.zoneID == "" {
		log.Info(fmt.Sprintf("Finding CloudFlare zone ID for %s", name))
		id, err := c.ZoneIDByName(name)
		if err != nil {
			return err
		}
		c.zoneID = id
	}
	log.Info(fmt.Sprintf("Checking Permissions on zone %s", c.zoneID))
	zone, err := c.ZoneDetails(c.zoneID)
	if err != nil {
		return err
	}
	if !strings.HasSuffix(name, "."+zone.Name) {
		return fmt.Errorf("CloudFlare zone name %q does not match name %q to be deployed", zone.Name, name)
	}
	needPerms := map[string]bool{"#zone:edit": false, "#zone:read": false}
	for _, perm := range zone.Permissions {
		if _, ok := needPerms[perm]; ok {
			needPerms[perm] = true
		}
	}
	for _, ok := range needPerms {
		if !ok {
			return fmt.Errorf("wrong permissions on zone %s: %v", c.zoneID, needPerms)
		}
	}
	return nil
}




func (c *cloudflareClient) uploadRecords(name string, records map[string]string) error {
	
	lrecords := make(map[string]string, len(records))
	for name, r := range records {
		lrecords[strings.ToLower(name)] = r
	}
	records = lrecords

	log.Info(fmt.Sprintf("Retrieving existing TXT records on %s", name))
	entries, err := c.DNSRecords(c.zoneID, cloudflare.DNSRecord{Type: "TXT"})
	if err != nil {
		return err
	}
	existing := make(map[string]cloudflare.DNSRecord)
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name, name) {
			continue
		}
		existing[strings.ToLower(entry.Name)] = entry
	}

	
	for path, val := range records {
		old, exists := existing[path]
		if !exists {
			
			log.Info(fmt.Sprintf("Creating %s = %q", path, val))
			ttl := rootTTL
			if path != name {
				ttl = treeNodeTTL 
			}
			_, err = c.CreateDNSRecord(c.zoneID, cloudflare.DNSRecord{Type: "TXT", Name: path, Content: val, TTL: ttl})
		} else if old.Content != val {
			
			log.Info(fmt.Sprintf("Updating %s from %q to %q", path, old.Content, val))
			old.Content = val
			err = c.UpdateDNSRecord(c.zoneID, old.ID, old)
		} else {
			log.Info(fmt.Sprintf("Skipping %s = %q", path, val))
		}
		if err != nil {
			return fmt.Errorf("failed to publish %s: %v", path, err)
		}
	}

	
	for path, entry := range existing {
		if _, ok := records[path]; ok {
			continue
		}
		
		log.Info(fmt.Sprintf("Deleting %s = %q", path, entry.Content))
		if err := c.DeleteDNSRecord(c.zoneID, entry.ID); err != nil {
			return fmt.Errorf("failed to delete %s: %v", path, err)
		}
	}
	return nil
}
