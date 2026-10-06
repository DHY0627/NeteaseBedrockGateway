package g79client

import (
	"archive/zip"
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

// G79ChatServerEntry represents a single chat server configuration returned by NetEase.
type G79ChatServerEntry struct {
	IP         string    `json:"IP"`
	IspEnabled Uncertain `json:"Isp_Enabled"`
	CTCCHost   string    `json:"ctcc_host"`
	CMCCHost   string    `json:"cmcc_host"`
	CUCCHost   string    `json:"cucc_host"`
	Port       int       `json:"Port"`
}

// G79LinkServerEntry 描述一个 Link 服务器节点。
type G79LinkServerEntry struct {
	Status     Uncertain `json:"status"`
	IspEnabled Uncertain `json:"Isp_Enabled"`
	IP         string    `json:"ip"`
	ServerType string    `json:"ServerType"`
	ID         Uncertain `json:"id"`
	Port       Uncertain `json:"port"`
}

// G79TransferServerEntry 描述一个传输服务器节点。
type G79TransferServerEntry struct {
	Status        Uncertain `json:"status"`
	IP            string    `json:"ip"`
	ISPEnabled    Uncertain `json:"Isp_Enabled"`
	BatchNew      Uncertain `json:"batchNew"`
	Batch         Uncertain `json:"batch"`
	ID            Uncertain `json:"id"`
	SignalWebPort Uncertain `json:"SignalWebPort"`
	ServerType    string    `json:"ServerType"`
	WebPort       Uncertain `json:"WebPort"`
	Ports         []int     `json:"ports"`
}

const (
	g79PackListURL  = "https://g79.update.netease.com/pack_list/production/g79_packlist_2"
	g79PatchListURL = "https://g79.update.netease.com/patch_list/production/g79_rn_patchlist"
)

// G79PackListEntry 描述单个平台的安装包信息。
type G79PackListEntry struct {
	URL     string `json:"url"`
	Text    string `json:"text"`
	Version string `json:"version"`
	MinVer  string `json:"min_ver"`
}

// PatchInfo describes the patch list payload exposed by NetEase.
type PatchInfo struct {
	IOS     []string `json:"ios"`
	Android []string `json:"android"`
	URL     string   `json:"url"`
	URLNew  string   `json:"urlNew"`
}

// PatchMetadata wraps the patch version and resources hash needed to reproduce the Android client handshake.
type PatchMetadata struct {
	Version       string
	ResourcesHash string
}

var (
	globalG79PatchMeta       *PatchMetadata
	globalG79ReleaseJSON     *G79ReleaseJSON
	globalX19ReleaseJSON     *X19ReleaseJSON
	globalG79ChatServers     []G79ChatServerEntry
	globalG79LinkServers     []G79LinkServerEntry
	globalG79TransferServers []G79TransferServerEntry
	globalG79PackList        map[string]G79PackListEntry

	g79PatchMu      sync.RWMutex
	g79ReleaseMu    sync.RWMutex
	x19ReleaseMu    sync.RWMutex
	g79ChatServerMu sync.RWMutex
	g79LinkMu       sync.RWMutex
	g79TransferMu   sync.RWMutex
	g79PackListMu   sync.RWMutex
)

func init() {
	// 在包初始化时尝试预取；失败则忽略，后续调用会再次尝试
	go func() {
		_, _ = GetGlobalG79LatestVersion()
		_, _ = GetGlobalG79ReleaseJSON()
		_, _ = GetGlobalX19ReleaseJSON()
		_, _ = GetGlobalG79ChatServers()
		_, _ = GetGlobalG79LinkServers()
		_, _ = GetGlobalG79TransferServers()
		_, _ = GetGlobalG79PackList()
	}()
}

func GetGlobalG79LatestVersion() (string, error) {
	meta, err := GetGlobalG79PatchMetadata()
	if err != nil {
		return "", err
	}
	return meta.Version, nil
}

// RefreshG79LatestVersion forces a refresh of the cached latest version.
func RefreshG79LatestVersion() (string, error) {
	meta, err := RefreshG79PatchMetadata()
	if err != nil {
		return "", err
	}
	return meta.Version, nil
}

func GetGlobalG79PatchMetadata() (*PatchMetadata, error) {
	g79PatchMu.RLock()
	if globalG79PatchMeta != nil {
		meta := *globalG79PatchMeta
		g79PatchMu.RUnlock()
		return &meta, nil
	}
	g79PatchMu.RUnlock()

	g79PatchMu.Lock()
	defer g79PatchMu.Unlock()
	if globalG79PatchMeta != nil {
		meta := *globalG79PatchMeta
		return &meta, nil
	}
	meta, err := fetchG79PatchMetadata()
	if err != nil {
		return nil, err
	}
	globalG79PatchMeta = meta
	return meta, nil
}

func RefreshG79PatchMetadata() (*PatchMetadata, error) {
	meta, err := fetchG79PatchMetadata()
	if err != nil {
		return nil, err
	}
	g79PatchMu.Lock()
	globalG79PatchMeta = meta
	g79PatchMu.Unlock()
	return meta, nil

}

func fetchG79PatchMetadata() (*PatchMetadata, error) {
	resp, err := http.Get(g79PatchListURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var patchInfo PatchInfo
	if err := json.Unmarshal(body, &patchInfo); err != nil {
		return nil, err
	}

	version, err := selectAndroidPatchVersion(patchInfo.Android)
	if err != nil {
		return nil, err
	}

	resourcesHash, err := fetchAndroidResourcesHash(version, patchInfo.URLNew)
	if err != nil {
		return nil, err
	}

	return &PatchMetadata{
		Version:       version,
		ResourcesHash: resourcesHash,
	}, nil
}

func selectAndroidPatchVersion(versions []string) (string, error) {
	if len(versions) == 0 {
		return "", fmt.Errorf("no android patch versions available")
	}
	base, err := getEngineBasePrefix()
	if err != nil {
		return "", err
	}
	for i := len(versions) - 1; i >= 0; i-- {
		if strings.HasPrefix(versions[i], base) {
			return versions[i], nil
		}
	}
	return "", fmt.Errorf("no android patch version matches base prefix %s", base)
}

func getEngineBasePrefix() (string, error) {
	parts := strings.Split(EngineVersion, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid engine version %q", EngineVersion)
	}
	return fmt.Sprintf("%s.%s", parts[0], parts[1]), nil
}

func fetchAndroidResourcesHash(version, baseURL string) (string, error) {
	if baseURL == "" {
		return "", fmt.Errorf("empty patch base url")
	}
	root := strings.TrimRight(baseURL, "/")
	pre := fmt.Sprintf("%s/android_%s/%s/android", root, version, version)
	manifest, err := fetchFirstZipEntry(fmt.Sprintf("%s/manifest.zip", pre))
	if err != nil {
		return "", err
	}
	var manifestPayload struct {
		Assets map[string]struct {
			MD5 string `json:"md5"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(manifest, &manifestPayload); err != nil {
		return "", err
	}

	hashBase := g79BaseMCPHash
	if asset, ok := manifestPayload.Assets["vanilla.mcp"]; ok && asset.MD5 != "" {
		hashBase = asset.MD5
	}
	if asset, ok := manifestPayload.Assets["vanilla_patch.mcp"]; ok && asset.MD5 != "" {
		hashBase += asset.MD5
	}

	rnURL := fmt.Sprintf("%s/rn/index.bundle.backup", pre)
	rnHash := ""
	if raw, err := downloadBytes(rnURL); err == nil {
		payload := raw
		if zipped, zipErr := readFirstFileFromZipData(raw); zipErr == nil {
			payload = zipped
		}
		rnHash = fmt.Sprintf("%x", md5.Sum(payload))
	}
	final := fmt.Sprintf("%x", md5.Sum([]byte(hashBase+rnHash)))
	return final, nil
}

func fetchFirstZipEntry(url string) ([]byte, error) {
	data, err := downloadBytes(url)
	if err != nil {
		return nil, err
	}
	return readFirstFileFromZipData(data)
}

func readFirstFileFromZipData(data []byte) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	if len(reader.File) == 0 {
		return nil, fmt.Errorf("zip archive is empty")
	}
	rc, err := reader.File[0].Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func downloadBytes(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s returned status %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func GetGlobalG79ReleaseJSON() (*G79ReleaseJSON, error) {
	g79ReleaseMu.RLock()
	if globalG79ReleaseJSON != nil {
		v := globalG79ReleaseJSON
		g79ReleaseMu.RUnlock()
		return v, nil
	}
	g79ReleaseMu.RUnlock()

	g79ReleaseMu.Lock()
	defer g79ReleaseMu.Unlock()
	if globalG79ReleaseJSON != nil {
		return globalG79ReleaseJSON, nil
	}
	v, err := fetchG79ReleaseJSON()
	if err != nil {
		return nil, err
	}
	globalG79ReleaseJSON = v
	return v, nil
}

// RefreshG79ReleaseJSON forces a refresh of the cached release JSON.
func RefreshG79ReleaseJSON() (*G79ReleaseJSON, error) {
	v, err := fetchG79ReleaseJSON()
	if err != nil {
		return nil, err
	}
	g79ReleaseMu.Lock()
	globalG79ReleaseJSON = v
	g79ReleaseMu.Unlock()
	return v, nil
}

func fetchG79ReleaseJSON() (*G79ReleaseJSON, error) {
	resp, err := http.Get("https://g79.update.netease.com/serverlist/adr_release.0.17.json")
	if err != nil {
		return nil, err
	}

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var g79ReleaseJSON G79ReleaseJSON
	if err := json.Unmarshal(body, &g79ReleaseJSON); err != nil {
		return nil, err
	}

	return &g79ReleaseJSON, nil
}

func GetGlobalX19ReleaseJSON() (*X19ReleaseJSON, error) {
	x19ReleaseMu.RLock()
	if globalX19ReleaseJSON != nil {
		v := globalX19ReleaseJSON
		x19ReleaseMu.RUnlock()
		return v, nil
	}
	x19ReleaseMu.RUnlock()

	x19ReleaseMu.Lock()
	defer x19ReleaseMu.Unlock()
	if globalX19ReleaseJSON != nil {
		return globalX19ReleaseJSON, nil
	}
	v, err := fetchX19ReleaseJSON()
	if err != nil {
		return nil, err
	}
	globalX19ReleaseJSON = v
	return v, nil
}

func RefreshX19ReleaseJSON() (*X19ReleaseJSON, error) {
	v, err := fetchX19ReleaseJSON()
	if err != nil {
		return nil, err
	}
	x19ReleaseMu.Lock()
	globalX19ReleaseJSON = v
	x19ReleaseMu.Unlock()
	return v, nil
}

func fetchX19ReleaseJSON() (*X19ReleaseJSON, error) {
	resp, err := http.Get("https://x19.update.netease.com/serverlist/release.json")
	if err != nil {
		return nil, err
	}

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var x19ReleaseJSON X19ReleaseJSON
	if err := json.Unmarshal(body, &x19ReleaseJSON); err != nil {
		return nil, err
	}

	return &x19ReleaseJSON, nil
}

// GetGlobalG79ChatServers returns the cached global chat server list, populating it on first use.
func GetGlobalG79ChatServers() ([]G79ChatServerEntry, error) {
	g79ChatServerMu.RLock()
	if globalG79ChatServers != nil {
		v := make([]G79ChatServerEntry, len(globalG79ChatServers))
		copy(v, globalG79ChatServers)
		g79ChatServerMu.RUnlock()
		return v, nil
	}
	g79ChatServerMu.RUnlock()

	g79ChatServerMu.Lock()
	defer g79ChatServerMu.Unlock()
	if globalG79ChatServers != nil {
		v := make([]G79ChatServerEntry, len(globalG79ChatServers))
		copy(v, globalG79ChatServers)
		return v, nil
	}

	list, err := fetchG79ChatServers()
	if err != nil {
		return nil, err
	}
	globalG79ChatServers = list
	return list, nil
}

// RefreshG79ChatServers forces a refresh of the cached chat server list.
func RefreshG79ChatServers() ([]G79ChatServerEntry, error) {
	list, err := fetchG79ChatServers()
	if err != nil {
		return nil, err
	}
	g79ChatServerMu.Lock()
	globalG79ChatServers = list
	g79ChatServerMu.Unlock()
	return list, nil
}

func fetchG79ChatServers() ([]G79ChatServerEntry, error) {
	g79ReleaseJSON, err := GetGlobalG79ReleaseJSON()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", g79ReleaseJSON.ChatServerURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("content-type", "text/plain")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var list []G79ChatServerEntry
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// GetGlobalG79LinkServers 获取全局 Link 服务器列表，如果已有缓存直接返回副本。
func GetGlobalG79LinkServers() ([]G79LinkServerEntry, error) {
	g79LinkMu.RLock()
	if globalG79LinkServers != nil {
		list := make([]G79LinkServerEntry, len(globalG79LinkServers))
		copy(list, globalG79LinkServers)
		g79LinkMu.RUnlock()
		return list, nil
	}
	g79LinkMu.RUnlock()

	g79LinkMu.Lock()
	defer g79LinkMu.Unlock()
	if globalG79LinkServers != nil {
		list := make([]G79LinkServerEntry, len(globalG79LinkServers))
		copy(list, globalG79LinkServers)
		return list, nil
	}

	list, err := fetchG79LinkServers()
	if err != nil {
		return nil, err
	}
	globalG79LinkServers = list
	return list, nil
}

// RefreshG79LinkServers 强制刷新缓存。
func RefreshG79LinkServers() ([]G79LinkServerEntry, error) {
	list, err := fetchG79LinkServers()
	if err != nil {
		return nil, err
	}
	g79LinkMu.Lock()
	globalG79LinkServers = list
	g79LinkMu.Unlock()
	return list, nil
}

func fetchG79LinkServers() ([]G79LinkServerEntry, error) {
	g79ReleaseJSON, err := GetGlobalG79ReleaseJSON()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", g79ReleaseJSON.LinkServerURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("content-type", "text/plain")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var list []G79LinkServerEntry
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// GetGlobalG79TransferServers 获取传输服务器列表（全局只拉取一次，后续复用缓存）。
func GetGlobalG79TransferServers() ([]G79TransferServerEntry, error) {
	g79TransferMu.RLock()
	if globalG79TransferServers != nil {
		v := make([]G79TransferServerEntry, len(globalG79TransferServers))
		copy(v, globalG79TransferServers)
		g79TransferMu.RUnlock()
		return v, nil
	}
	g79TransferMu.RUnlock()

	g79TransferMu.Lock()
	defer g79TransferMu.Unlock()
	if globalG79TransferServers != nil {
		v := make([]G79TransferServerEntry, len(globalG79TransferServers))
		copy(v, globalG79TransferServers)
		return v, nil
	}

	list, err := fetchG79TransferServers()
	if err != nil {
		return nil, err
	}
	globalG79TransferServers = list
	return list, nil
}

// RefreshG79TransferServers 强制刷新传输服务器列表缓存。
func RefreshG79TransferServers() ([]G79TransferServerEntry, error) {
	list, err := fetchG79TransferServers()
	if err != nil {
		return nil, err
	}
	g79TransferMu.Lock()
	globalG79TransferServers = list
	g79TransferMu.Unlock()
	return list, nil
}

func fetchG79TransferServers() ([]G79TransferServerEntry, error) {
	g79ReleaseJSON, err := GetGlobalG79ReleaseJSON()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", g79ReleaseJSON.TransferServerUrl, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("content-type", "text/plain")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var list []G79TransferServerEntry
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// GetGlobalG79PackList 获取全局 g79 packlist 配置，首次调用会自动拉取并缓存。
func GetGlobalG79PackList() (map[string]G79PackListEntry, error) {
	g79PackListMu.RLock()
	if globalG79PackList != nil {
		v := cloneG79PackList(globalG79PackList)
		g79PackListMu.RUnlock()
		return v, nil
	}
	g79PackListMu.RUnlock()

	g79PackListMu.Lock()
	defer g79PackListMu.Unlock()
	if globalG79PackList != nil {
		return cloneG79PackList(globalG79PackList), nil
	}

	list, err := fetchG79PackList()
	if err != nil {
		return nil, err
	}
	globalG79PackList = list
	return cloneG79PackList(list), nil
}

// RefreshG79PackList 强制刷新 g79 packlist 缓存。
func RefreshG79PackList() (map[string]G79PackListEntry, error) {
	list, err := fetchG79PackList()
	if err != nil {
		return nil, err
	}
	g79PackListMu.Lock()
	globalG79PackList = list
	g79PackListMu.Unlock()
	return cloneG79PackList(list), nil
}

func fetchG79PackList() (map[string]G79PackListEntry, error) {
	req, err := http.NewRequest("GET", g79PackListURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("content-type", "text/plain")
	req.Header.Set("cache-control", "no-cache")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var list map[string]G79PackListEntry
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, err
	}
	return list, nil
}

func cloneG79PackList(src map[string]G79PackListEntry) map[string]G79PackListEntry {
	if src == nil {
		return nil
	}
	dst := make(map[string]G79PackListEntry, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
