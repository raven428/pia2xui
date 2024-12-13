package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"
	"golang.org/x/crypto/curve25519"
	_ "modernc.org/sqlite"
)

type WireGuardResponse struct {
	Status     string   `json:"status"`
	ServerKey  string   `json:"server_key"`
	ServerPort int      `json:"server_port"`
	ServerIP   string   `json:"server_ip"`
	PeerIP     string   `json:"peer_ip"`
	DNSServers []string `json:"dns_servers"`
}

type XrayTemplateConfig struct {
	MTU            int      `json:"mtu"`
	SecretKey      string   `json:"secretKey"`
	Address        []string `json:"address"`
	Workers        int      `json:"workers"`
	DomainStrategy string   `json:"domainStrategy"`
	Peers          []Peer   `json:"peers"`
	KernelMode     bool     `json:"kernelMode"`
	KernelTun      bool     `json:"kernelTun"`
}

type Peer struct {
	PublicKey  string   `json:"publicKey"`
	AllowedIPs []string `json:"allowedIPs"`
	Endpoint   string   `json:"endpoint"`
	KeepAlive  int      `json:"keepAlive"`
}

// Генерация приватного и публичного ключей WireGuard
func generateKeys() (string, string, error) {
	// Генерация приватного ключа как среза
	privateKeySlice := make([]byte, 32)
	_, err := rand.Read(privateKeySlice)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate private key: %v", err)
	}

	// Преобразуем в массив и маскируем
	var privateKey [32]byte
	copy(privateKey[:], privateKeySlice)
	privateKey[0] &= 248
	privateKey[31] &= 127
	privateKey[31] |= 64

	// Вычисляем публичный ключ
	var publicKey [32]byte
	curve25519.ScalarBaseMult(&publicKey, &privateKey)

	// Кодируем ключи в Base64
	privateKeyBase64 := base64.StdEncoding.EncodeToString(privateKey[:])
	publicKeyBase64 := base64.StdEncoding.EncodeToString(publicKey[:])

	return privateKeyBase64, publicKeyBase64, nil
}

func getServerInfo(regionID string) (string, string, error) {
	resp, err := http.Get("https://serverlist.piaservers.net/vpninfo/servers/v6")
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch server info: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("failed to decode server info: %v", err)
	}

	for _, region := range result["regions"].([]interface{}) {
		r := region.(map[string]interface{})
		if r["id"].(string) == regionID {
			server := r["servers"].(map[string]interface{})["wg"].([]interface{})[0].(map[string]interface{})
			return server["ip"].(string), server["cn"].(string), nil
		}
	}

	return "", "", fmt.Errorf("region %s not found", regionID)
}

func getPiaToken(username, password string) (string, error) {
	data := fmt.Sprintf("username=%s&password=%s", username, password)
	resp, err := http.Post("https://www.privateinternetaccess.com/api/client/v2/token",
		"application/x-www-form-urlencoded", bytes.NewBufferString(data))
	if err != nil {
		return "", fmt.Errorf("failed to fetch token: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode token response: %v", err)
	}

	return result["token"].(string), nil
}

func addKey(cn, ip, piaToken, publicKey, certPath string) (*WireGuardResponse, error) {
	// Настраиваем кастомный транспорт для перенаправления подключения
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs:            x509.NewCertPool(),
			InsecureSkipVerify: false, // Оставляем проверку сертификата
		},
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			// Заменяем хост на IP при подключении
			if addr == fmt.Sprintf("%s:1337", cn) {
				addr = fmt.Sprintf("%s:1337", ip)
			}
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
	}

	// Добавляем CA-файл
	if certPath != "" {
		caCert, err := os.ReadFile(certPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %v", err)
		}
		if !transport.TLSClientConfig.RootCAs.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to append CA certificate to pool")
		}
	}

	// Создаём HTTP-клиент
	client := &http.Client{
		Transport: transport,
	}

	// Формируем URL
	url := fmt.Sprintf("https://%s:1337/addKey", cn)

	// Настраиваем параметры запроса
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Добавляем параметры к URL
	q := req.URL.Query()
	q.Add("pt", piaToken)
	q.Add("pubkey", publicKey)
	req.URL.RawQuery = q.Encode()

	// Выполняем запрос
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Проверяем статус ответа
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected response: %d - %s", resp.StatusCode, string(body))
	}

	// Парсим JSON-ответ
	var result WireGuardResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response JSON: %v", err)
	}

	return &result, nil
}

func updateXrayTemplateConfig(dbPath, tag string, newSettings XrayTemplateConfig) error {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %v", err)
	}
	defer db.Close()

	// 1. Получение текущего значения из базы данных
	var currentConfigJSON string
	query := "SELECT value FROM settings WHERE key = 'xrayTemplateConfig'"
	err = db.QueryRow(query).Scan(&currentConfigJSON)
	if err != nil {
		return fmt.Errorf("failed to fetch current config: %v", err)
	}

	// 2. Парсинг текущего JSON
	var currentConfig map[string]interface{}
	err = json.Unmarshal([]byte(currentConfigJSON), &currentConfig)
	if err != nil {
		return fmt.Errorf("failed to parse current config JSON: %v", err)
	}

	// 3. Поиск и изменение нужного outbounds
	outbounds, ok := currentConfig["outbounds"].([]interface{})
	if !ok {
		return fmt.Errorf("invalid or missing 'outbounds' field in config")
	}

	found := false
	for _, outbound := range outbounds {
		outboundMap, ok := outbound.(map[string]interface{})
		if !ok {
			continue
		}

		if outboundMap["tag"] == tag {
			outboundMap["settings"] = newSettings
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("tag '%s' not found in 'outbounds'", tag)
	}

	// 4. Конвертирование изменённого JSON обратно
	modifiedConfigJSON, err := json.Marshal(currentConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal modified config JSON: %v", err)
	}

	// 5. Обновление базы данных
	_, err = db.Exec("UPDATE settings SET value = ? WHERE key = 'xrayTemplateConfig'", string(modifiedConfigJSON))
	if err != nil {
		return fmt.Errorf("failed to update config in database: %v", err)
	}

	return nil
}

func manageService(action, serviceName string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 11*time.Second)
	defer cancel()

	conn, err := dbus.NewSystemdConnectionContext(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to connect to systemd: %v", err)
	}
	defer conn.Close()

	switch action {
	case "start":
		_, err = conn.StartUnitContext(ctx, serviceName, "replace", nil)
		return false, err
	case "stop":
		_, err = conn.StopUnitContext(ctx, serviceName, "replace", nil)
		return false, err
	case "is-active":
		unitStatus, err := conn.GetUnitPropertyContext(ctx, serviceName, "ActiveState")
		if err != nil {
			return false, fmt.Errorf("failed to get service status: %v", err)
		}
		if strings.Trim(unitStatus.Value.String(), "\"") == "active" {
			return true, nil
		}
		return false, nil
	default:
		return false, fmt.Errorf("invalid action: %s", action)
	}
}

func main() {
	// Параметры командной строки
	username := flag.String("username", "", "PIA username")
	password := flag.String("password", "", "PIA password")
	regionID := flag.String("region", "fi", "Region ID for server info")
	tag := flag.String("tag", "wg-proton-tr23", "Tag for xray config")
	certPath := flag.String("cert", "ca.rsa.4096.crt", "Path to the CA certificate")
	dbPath := flag.String("db", "x-ui.db", "Path to the SQLite database")
	serviceName := flag.String("service", "x-ui.service", "Service name for systemd")

	flag.Parse()

	// Проверка обязательных параметров
	if *username == "" || *password == "" {
		log.Fatal("username and password are required")
	}

	privateKey, publicKey, err := generateKeys()
	if err != nil {
		log.Fatalf("%v", err)
	}
	serverIP, serverCN, err := getServerInfo(*regionID)
	if err != nil {
		log.Fatalf("%v", err)
	}
	token, err := getPiaToken(*username, *password)
	if err != nil {
		log.Fatalf("PIA token fatal: %v", err)
	}
	wgResp, err := addKey(serverCN, serverIP, token, publicKey, *certPath)
	if err != nil {
		log.Fatalf("AddWG fatal: %v", err)
	}
	log.Printf("WireGuard key added successfully: %+v", wgResp)

	config := XrayTemplateConfig{
		MTU:            1420,
		SecretKey:      privateKey,
		Address:        []string{fmt.Sprintf("%s/32", wgResp.PeerIP)},
		Workers:        2,
		DomainStrategy: "ForceIPv4",
		Peers: []Peer{
			{
				PublicKey:  wgResp.ServerKey,
				AllowedIPs: []string{"0.0.0.0/0", "::/0"},
				Endpoint:   fmt.Sprintf("%s:%d", wgResp.ServerIP, wgResp.ServerPort),
				KeepAlive:  13,
			},
		},
		KernelMode: false,
		KernelTun:  false,
	}

	active, err := manageService("is-active", *serviceName)
	if err != nil {
		log.Printf("Failed to check service status: %v", err)
	}

	if active {
		log.Printf("Stopping service: %s", *serviceName)
		_, err := manageService("stop", *serviceName)
		if err != nil {
			log.Printf("Failed to stop service: %v", err)
		}
	}

	updateXrayTemplateConfig(*dbPath, *tag, config)

	if active {
		log.Printf("Starting service: %s", *serviceName)
		manageService("start", *serviceName)
	}
}
