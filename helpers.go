package leaseweb

import (
	"fmt"
	"time"

	"github.com/libdns/libdns"
)

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

		newRecordSet.Name = newRecordSet.Name + "."
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
