package leaseweb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
)

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
