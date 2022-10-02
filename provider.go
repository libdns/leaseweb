// Package leaseweb implements a DNS record management client compatible
// with the libdns interfaces for Leaseweb.
// Upstream documentation found at:
// https://developer.leaseweb.com/api-docs/domains_v2.html
package leaseweb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/libdns/libdns"
)

const (
	LeasewebApiKeyHeader = "X-LSW-Auth"
)

// Provider facilitates DNS record manipulation with Leaseweb.
type Provider struct {
	// Leasewebs API key. Generate one in the Leaseweb customer portal -> Administration -> API Key
	APIKey string `json:"api_token,omitempty"`
	mutex  sync.Mutex
}

func fromLibdns(zone string, records []libdns.Record) ([]leasewebRecordSet, error) {
	var recordsInfo = []struct {
		libdnsRecord libdns.Record
		consumed     bool
	}{}
	for _, record := range records {
		recordsInfo = append(recordsInfo, struct {
			libdnsRecord libdns.Record
			consumed     bool
		}{
			libdnsRecord: record,
			consumed:     false,
		})
	}

	var errors []string
	var recordSets []leasewebRecordSet

	for currentIdx := 0; currentIdx < len(recordsInfo); currentIdx++ {
		var currentRecordInfo = &recordsInfo[currentIdx]

		if currentRecordInfo.consumed {
			continue
		}
		currentRecordInfo.consumed = true

		var newRecordSet = leasewebRecordSet{
			Name:    currentRecordInfo.libdnsRecord.Name,
			Type:    currentRecordInfo.libdnsRecord.Type,
			TTL:     int(currentRecordInfo.libdnsRecord.TTL.Seconds()),
			Content: []string{currentRecordInfo.libdnsRecord.Value},
		}

		for otherIdx := 0; otherIdx < len(recordsInfo); otherIdx++ {
			var otherRecordInfo = &recordsInfo[otherIdx]
			if otherIdx == currentIdx {
				continue
			}

			if otherRecordInfo.libdnsRecord.Name == newRecordSet.Name && otherRecordInfo.libdnsRecord.Type == currentRecordInfo.libdnsRecord.Type {
				otherRecordInfo.consumed = true

				var otherTTL = int(otherRecordInfo.libdnsRecord.TTL.Seconds())
				if otherTTL != newRecordSet.TTL {
					errors = append(errors, fmt.Sprintf("Found different TTL values for %s: %d and %d.", newRecordSet.Name, newRecordSet.TTL, otherTTL))
				}

				newRecordSet.Content = append(newRecordSet.Content, otherRecordInfo.libdnsRecord.Value)
			}
		}
		recordSets = append(recordSets, newRecordSet)
	}

	if len(errors) > 0 {
		return nil, fmt.Errorf("%v", errors)
	}

	return recordSets, nil
}

func fromLeaseweb(recordSets leasewebRecordSets) []libdns.Record {
	var records []libdns.Record
	for _, resourceRecordSet := range recordSets.ResourceRecordSets {
		for _, content := range resourceRecordSet.Content {
			record := libdns.Record{
				Name:  resourceRecordSet.Name,
				Value: content,
				Type:  resourceRecordSet.Type,
				TTL:   time.Duration(resourceRecordSet.TTL) * time.Second,
			}
			records = append(records, record)
		}
	}
	return records
}

func (p *Provider) getRecordsHTTP(domainName string) (leasewebRecordSets, error) {
	httpClient := &http.Client{}

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://api.leaseweb.com/hosting/v2/domains/%s/resourceRecordSets", domainName), nil)
	if err != nil {
		return leasewebRecordSets{}, err
	}

	req.Header.Add(LeasewebApiKeyHeader, p.APIKey)

	res, err := httpClient.Do(req)
	defer res.Body.Close()
	if err != nil {
		return leasewebRecordSets{}, err
	}
	// if res.StatusCode == 401 {
	// 	return nil, fmt.Errorf("Received StatusCode %d from Leaseweb API, used APIKey: %s", res.StatusCode, p.APIKey)
	// }
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return leasewebRecordSets{}, fmt.Errorf("Received StatusCode %d from Leaseweb API.", res.StatusCode)
	}

	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return leasewebRecordSets{}, err
	}

	var recordSets leasewebRecordSets
	json.Unmarshal([]byte(data), &recordSets)

	return recordSets, nil
}

// GetRecords lists all the records in the zone.
func (p *Provider) GetRecords(ctx context.Context, zone string) ([]libdns.Record, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	domainName := strings.TrimSuffix(zone, ".")

	recordSets, err := p.getRecordsHTTP(domainName)
	if err != nil {
		return nil, err
	}

	records := fromLeaseweb(recordSets)

	return records, nil
}

func (p *Provider) postToResourceRecordSet(zone string, recordSet leasewebRecordSet) (leasewebRecordSet, error) {
	client := &http.Client{}

	bodyBuffer := new(bytes.Buffer)
	json.NewEncoder(bodyBuffer).Encode(recordSet)

	var domainName = strings.TrimSuffix(zone, ".")

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("https://api.leaseweb.com/hosting/v2/domains/%s/resourceRecordSets", domainName), bodyBuffer)
	if err != nil {
		return leasewebRecordSet{}, err
	}

	req.Header.Add(LeasewebApiKeyHeader, p.APIKey)

	res, err := client.Do(req)
	defer res.Body.Close()
	if err != nil {
		return leasewebRecordSet{}, err
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return leasewebRecordSet{}, fmt.Errorf("Received StatusCode %d from Leaseweb API.", res.StatusCode)
	}

	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return leasewebRecordSet{}, err
	}

	json.Unmarshal([]byte(data), &recordSet)
	return recordSet, nil
}

func (p *Provider) putToResourceRecordSet(domainName string, recordSet leasewebRecordSet) (leasewebRecordSets, error) {
	client := &http.Client{}

	bodyBuffer := new(bytes.Buffer)
	json.NewEncoder(bodyBuffer).Encode(&updateRecordSetRequest{
		Content: recordSet.Content,
		TTL:     recordSet.TTL,
	})

	// https://developer.leaseweb.com/api-docs/domains_v2.html#operation/put/domains/{domainName}/resourceRecordSets/{name}/{type}
	// https://api.leaseweb.com/hosting/v2/domains/{domainName}/resourceRecordSets/{name}/{type}
	req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("https://api.leaseweb.com/hosting/v2/domains/%s/resourceRecordSets/%s/%s", domainName, recordSet.Name, recordSet.Type), bodyBuffer)
	if err != nil {
		return leasewebRecordSets{}, err
	}
	req.Header.Add(LeasewebApiKeyHeader, p.APIKey)

	res, err := client.Do(req)
	defer res.Body.Close()
	if err != nil {
		return leasewebRecordSets{}, err
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {

		return leasewebRecordSets{}, fmt.Errorf("Received StatusCode %d from Leaseweb API. %s", res.StatusCode, res.Body)
	}

	return leasewebRecordSets{}, nil
}

// AppendRecords adds records to the zone. It returns the records that were added.
func (p *Provider) AppendRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	recordSets, err := fromLibdns(zone, records)
	if err != nil {
		return nil, err
	}

	for _, recordSet := range recordSets {
		_, err := p.postToResourceRecordSet(zone, recordSet)
		if err != nil {
			return nil, err
		}
	}

	// TODO: Ideally should check which records are actually POSTed.
	// For now we can assume all if we reach this point with no errors.
	var addedRecords = records
	return addedRecords, nil
}

// SetRecords sets the records in the zone, either by updating existing records or creating new ones.
// It returns the updated records.
func (p *Provider) SetRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	domainName := strings.TrimSuffix(zone, ".")
	existingRecordSets, err := p.getRecordsHTTP(domainName)
	if err != nil {
		return nil, err
	}

	recordSets, err := fromLibdns(zone, records)
	if err != nil {
		return nil, err
	}

	existingRecords := fromLeaseweb(existingRecordSets)

	var updatedRecords []libdns.Record
	for _, recordSet := range recordSets {
		var hasExisting = false
		for _, existingRecord := range existingRecords {
			if existingRecord.Name == recordSet.Name && existingRecord.Type == recordSet.Type {
				hasExisting = true
			}
		}

		if hasExisting {
			updatedRecordResponse, err := p.putToResourceRecordSet(zone, recordSet)
			if err != nil {
				return nil, err
			}

			for _, updatedRecord := range fromLeaseweb(updatedRecordResponse) {
				updatedRecords = append(updatedRecords, updatedRecord)
			}
		} else {
			_, err := p.postToResourceRecordSet(zone, recordSet)
			if err != nil {
				return nil, err
			}
		}
	}

	return updatedRecords, nil
}

// DeleteRecords deletes the records from the zone. It returns the records that were deleted.
// Leaseweb specifics:
// - Well-formatted DELETE requests will always succeed, even for non-existing records.
func (p *Provider) DeleteRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	client := &http.Client{}

	var domainName = strings.TrimSuffix(zone, ".")

	recordSets, err := fromLibdns(zone, records)

	for _, recordSet := range recordSets {
		if err != nil {
			return nil, err
		}

		// https://developer.leaseweb.com/api-docs/domains_v2.html#operation/delete/domains/{domainName}/resourceRecordSets/{name}/{type}
		req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("https://api.leaseweb.com/hosting/v2/domains/%s/resourceRecordSets/%s/%s", domainName, recordSet.Name, recordSet.Type), nil)
		if err != nil {
			return nil, err
		}

		req.Header.Add(LeasewebApiKeyHeader, p.APIKey)

		res, err := client.Do(req)
		defer res.Body.Close()
		if err != nil {
			return nil, err
		}
		if res.StatusCode < 200 || res.StatusCode > 299 {
			return nil, fmt.Errorf("Received StatusCode %d from Leaseweb API.", res.StatusCode)
		}
	}

	// TODO: Ideally should check which records are actually POSTed.
	// For now we can assume all if we reach this point with no errors.
	var deletedRecords = records
	return deletedRecords, nil
}

// Interface guards
var (
	_ libdns.RecordGetter   = (*Provider)(nil)
	_ libdns.RecordAppender = (*Provider)(nil)
	_ libdns.RecordSetter   = (*Provider)(nil)
	_ libdns.RecordDeleter  = (*Provider)(nil)
)
