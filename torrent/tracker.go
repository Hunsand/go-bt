package torrent

import (
	"crypto/rand"
	"demo/bencode"
	"encoding/binary"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	PeerPort int = 6666
	IpLen    int = 4
	PortLen  int = 2
	PeerLen  int = IpLen + PortLen
)

const IDLEN int = 20

type PeerInfo struct {
	Ip   net.IP
	Port uint16
}

type TrackerResp struct {
	Interval int    `bencode:"interval"`
	Peers    string `bencode:"peers"`
}

func FindPeers(tf *TorrentFile) []PeerInfo {

	url, err := buildURL(tf)
	if err != nil {
		fmt.Println("Build URL error: " + err.Error())
		return nil
	}

	cli := &http.Client{Timeout: 15 * time.Second}
	resp, err := cli.Get(url)
	if err != nil {
		fmt.Println("http get request error: " + err.Error())
		return nil
	}
	defer resp.Body.Close()

	trackerResp := new(TrackerResp)
	err = bencode.Unmarshal(resp.Body, trackerResp)
	if err != nil {
		fmt.Println("[FindPeers] unmarshal error: " + err.Error())
		return nil
	}

	return buildPeerInfo([]byte(trackerResp.Peers))
}

func buildURL(tf *TorrentFile) (string, error) {

	// peer的唯一标识，简单随机生成
	var peerID [20]byte
	_, err := rand.Read(peerID[:])
	if err != nil {
		return "", err
	}
	base, err := url.Parse(tf.Announce)
	if err != nil {
		fmt.Println("Announce Error: " + tf.Announce)
		return "", err
	}
	params := url.Values{
		"info_hash":  []string{string(tf.InfoSHA[:])},  // 文件标识
		"peer_id":    []string{string(peerID[:])},      // peer标识
		"port":       []string{strconv.Itoa(PeerPort)}, // 端口
		"uploaded":   []string{"0"},
		"downloaded": []string{"0"},
		"compact":    []string{"1"},
		"left":       []string{strconv.Itoa(tf.FileLen)}, // 剩余下载大小
	}
	// 构建URL查询参数，例如
	// http://bttracker.debian.org:6969/announce?info_hash=xxxxx&peer_id=yyyyy&port=6666&uploaded=0&downloaded=0&compact=1&left=5242880
	base.RawQuery = params.Encode()

	return base.String(), nil
}

func buildPeerInfo(peers []byte) []PeerInfo {
	// 计算peer数量，每个peer占用6字节(4字节IP + 2字节端口)
	num := len(peers) / PeerLen // PeerLen = 6

	// 验证数据格式是否正确（总字节数必须是6的倍数）
	if len(peers)%PeerLen != 0 {
		fmt.Println("Received Malformed peers")
		return nil
	}

	// 创建PeerInfo切片，长度为peer数量
	infos := make([]PeerInfo, num)
	for i := range num {
		// 计算当前peer的起始位置
		offset := i * PeerLen
		// 提取IP地址（前4个字节）
		infos[i].Ip = net.IP(peers[offset : offset+IpLen])
		// 提取端口号（后2个字节），使用大端序解析
		infos[i].Port = binary.BigEndian.Uint16(peers[offset+IpLen : offset+PeerLen])
	}

	return infos
}
