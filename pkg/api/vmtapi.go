package api

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/golang/glog"
)

type VmtApi struct {
	vmtUrl    string
	extConfig map[string]string
}

const (
	logger = "VMTurbo API"
)

func (vmtApi *VmtApi) Post(postUrl, requestDataString string) (string, error) {
	return vmtApi.apiPost(postUrl, requestDataString)
}

func (vmtApi *VmtApi) Get(getUrl string) (string, error) {
	return vmtApi.apiGet(getUrl)
}

func (vmtApi *VmtApi) Delete(getUrl string) (string, error) {
	return vmtApi.apiDelete(getUrl)
}

// Call vmturbo api. return response
func (vmtApi *VmtApi) apiPost(postUrl, requestDataString string) (string, error) {
	fullUrl := vmtApi.vmtUrl + "/vmturbo/api" + postUrl + requestDataString
	glog.V(4).Info("The full Url is ", fullUrl)
	req, err := http.NewRequest("POST", fullUrl, nil)

	req.SetBasicAuth(vmtApi.extConfig["Username"], vmtApi.extConfig["Password"])
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	resp, err := client.Do(req)
	if err != nil {
		glog.Errorf("Error getting response: %s", err)
		return "", err
	}

	respContent, err := parseAPICallResponse(resp)
	if err != nil {
		glog.Errorf("Error getting response: %s", err)
		return "", err
	}
	if resp.StatusCode != 200 {
		glog.Errorf("Response failed: %s", resp.Status)
		return "", err
	}
	glog.V(4).Infof("Post Succeed: %s", string(respContent))

	defer resp.Body.Close()
	return respContent, nil
}

// Call vmturbo api. return response
func (vmtApi *VmtApi) apiGet(getUrl string) (string, error) {
	fullUrl := vmtApi.vmtUrl + "/vmturbo/api" + getUrl
	glog.V(4).Infof("The full Url is %s", fullUrl)
	req, err := http.NewRequest("GET", fullUrl, nil)

	req.SetBasicAuth(vmtApi.extConfig["Username"], vmtApi.extConfig["Password"])
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}

	resp, err := client.Do(req)
	if err != nil {
		glog.Errorf("Error getting response: %s", err)
		return "", err
	}
	respContent, err := parseAPICallResponse(resp)
	if err != nil {
		glog.Errorf("Error getting response: %s", err)
		return "", err
	}
	if resp.StatusCode != 200 {
		glog.Errorf("Response failed: %s", resp.Status)
		return "", err
	}
	glog.V(4).Infof("Get Succeed: %s", string(respContent))
	defer resp.Body.Close()
	return respContent, nil
}

// Delete API call
func (vmtApi *VmtApi) apiDelete(getUrl string) (string, error) {
	fullUrl := vmtApi.vmtUrl + "/vmturbo/api" + getUrl
	glog.V(4).Infof("The full Url is ", fullUrl)
	req, err := http.NewRequest("DELETE", fullUrl, nil)

	req.SetBasicAuth(vmtApi.extConfig["Username"], vmtApi.extConfig["Password"])
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	resp, err := client.Do(req)
	if err != nil {
		glog.Errorf("Error getting response: %s", err)
		return "", err
	}
	respContent, err := parseAPICallResponse(resp)
	if err != nil {
		glog.Errorf("Error getting response: %s", err)
		return "", err
	}
	if resp.StatusCode != 200 {
		glog.Errorf("Response failed: %s", resp.Status)
		return "", err
	}
	glog.V(4).Infof("DELETE call Succeed: %s", string(respContent))
	defer resp.Body.Close()
	return respContent, nil
}

// this method takes in a reservation response and should return the reservation uuid, if there is any
func parseAPICallResponse(resp *http.Response) (string, error) {
	if resp == nil {
		return "", fmt.Errorf("response sent in is nil")
	}
	glog.V(4).Infof("response body is %s", resp.Body)

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		glog.Errorf("Error after ioutil.ReadAll: %s", err)
		return "", err
	}
	glog.V(4).Infof("response content is %s", string(content))

	return string(content), nil
}

func NewVmtApi(url string, externalConfiguration map[string]string) *VmtApi {
	return &VmtApi{
		vmtUrl:    url,
		extConfig: externalConfiguration,
	}
}
