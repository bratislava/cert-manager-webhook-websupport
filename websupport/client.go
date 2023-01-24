package websupport

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type DnsRecord struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Ttl     int    `json:"ttl"`
	Id      int    `json:"id"`
}

type Domains struct {
	Items []DnsRecord `json:"items"`
}

type Config struct {
	ApiKey    string
	ApiSecret string
}

type Client struct {
	httpClient *http.Client
	Config     *Config
}

type HttpErrorContent struct {
	Content []string `json:"content"`
}

type WebsupportError struct {
	Item   DnsRecord        `json:"item"`
	Status string           `json:"status"`
	Errors HttpErrorContent `json:"errors"`
}

func (e *WebsupportError) Error() string {
	return e.Errors.Content[0]
}

func secretSignature(method string, fullUrl string, secret string) (string, error) {
	url, err := url.Parse(fullUrl)
	if err != nil {
		return "", err
	}

	h := hmac.New(sha1.New, []byte(secret))
	h.Write([]byte(fmt.Sprintf("%s %s %d", method, url.Path, time.Now().Unix())))

	signature := fmt.Sprintf("%x", h.Sum(nil))
	return signature, nil
}

func NewClient(config *Config) *Client {
	httpClient := http.Client{
		Timeout: 10 * time.Second,
	}

	return &Client{
		httpClient: &httpClient,
		Config:     config,
	}
}

func (client *Client) BaseUrl() string {
	return "https://rest.websupport.sk/v1/user/self/zone/"
}

func (client *Client) NewRequest(method string, url string, data []byte) (*http.Request, error) {
	req, err := http.NewRequest(method, url, bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Date", time.Now().Format(time.RFC3339))

	signature, err := secretSignature(method, url, client.Config.ApiSecret)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(client.Config.ApiKey, signature)
	return req, nil
}

func (client *Client) Request(method string, url string, data []byte) (*http.Response, error) {
	req, err := client.NewRequest(method, url, data)
	if err != nil {
		return nil, err
	}

	resp, err := client.httpClient.Do(req)

	if resp.StatusCode >= 400 {
		var wsError WebsupportError
		json.NewDecoder(resp.Body).Decode(&wsError)
		wsError.Status = resp.Status
		return nil, &wsError
	}
	return resp, err
}

func (client *Client) GetDNSRecords(domainName string) (*Domains, error) {
	var url string = client.BaseUrl() + domainName + "/record"

	resp, err := client.Request("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var domains Domains
	err = json.NewDecoder(resp.Body).Decode(&domains)
	if err != nil {
		return nil, err
	}
	return &domains, err
}

func (client *Client) FindDNSRecord(domainName string, dnsRecord *DnsRecord) (*DnsRecord, error) {
	records, err := client.GetDNSRecords(domainName)
	if err != nil {
		return nil, err
	}

	for _, record := range records.Items {
		if record.Name == dnsRecord.Name && record.Type == dnsRecord.Type &&
			(dnsRecord.Content == "" || record.Content == dnsRecord.Content) &&
			(dnsRecord.Id == 0 || record.Id == dnsRecord.Id) &&
			(dnsRecord.Ttl == 0 || record.Ttl == dnsRecord.Ttl) {
			return &record, nil
		}
	}

	err_msg := fmt.Sprintf("no such domain '%v' with key '%s' found", dnsRecord.Name, dnsRecord.Content)
	return nil, &WebsupportError{
		Item:   *dnsRecord,
		Errors: HttpErrorContent{Content: []string{err_msg}},
	}
}

func (client *Client) CreateRecord(domainName string, dnsRecord *DnsRecord) error {
	var url string = client.BaseUrl() + domainName + "/record"

	data, err := json.Marshal(dnsRecord)
	if err != nil {
		return err
	}

	resp, err := client.Request("POST", url, data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

func (client *Client) UpdateRecord(domainName string, oldDnsRecord *DnsRecord, newDnsRecord *DnsRecord) error {
	foundRecord, err := client.FindDNSRecord(domainName, oldDnsRecord)
	if err != nil {
		return err
	}

	data, err := json.Marshal(newDnsRecord)
	if err != nil {
		return err
	}

	var url string = fmt.Sprintf("%s%s/record/%d", client.BaseUrl(), domainName, foundRecord.Id)
	resp, err := client.Request("PUT", url, data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

func (client *Client) DeleteRecord(domainName string, dnsRecord *DnsRecord) error {
	foundRecord, err := client.FindDNSRecord(domainName, dnsRecord)
	if err != nil {
		return err
	}
	var url string = fmt.Sprintf("%s%s/record/%d", client.BaseUrl(), domainName, foundRecord.Id)
	resp, err := client.Request("DELETE", url, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}
