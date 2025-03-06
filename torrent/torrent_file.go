package torrent

import (
	"bytes"
	"crypto/sha1"
	"demo/bencode"
	"fmt"
	"io"
)

type rawInfo struct {
	Length      int    `bencode:"length"`
	Name        string `bencode:"name"`
	PieceLength int    `bencode:"piece length"`
	Pieces      string `bencode:"pieces"`
}

type rawFile struct {
	Announce string  `bencode:"announce"`
	Info     rawInfo `bencode:"info"`
}

const SHALEN int = 20

// 打平之后的rawFile
type TorrentFile struct {
	Announce string       // Tracker URL
	InfoSHA  [SHALEN]byte // 文件的唯一标识
	FileName string
	FileLen  int
	PieceLen int
	PieceSHA [][SHALEN]byte
}

// 转化为TorrentFile Struct格式
func ParseFile(r io.Reader) (*TorrentFile, error) {
	raw := new(rawFile)
	err := bencode.Unmarshal(r, raw)
	if err != nil {
		fmt.Println("Fail to parse torrent file")
		return nil, err
	}

	res := &TorrentFile{
		Announce: raw.Announce,
		FileName: raw.Info.Name,
		FileLen:  raw.Info.Length,
		PieceLen: raw.Info.PieceLength,
	}

	// 计算InfoSHA
	buf := new(bytes.Buffer)
	wLen := bencode.Marshal(buf, raw.Info)
	if wLen == 0 {
		fmt.Println("raw file info error")
	}
	res.InfoSHA = sha1.Sum(buf.Bytes())

	// 计算PiecesSHA
	bys := []byte(raw.Info.Pieces)
	cnt := len(bys) / SHALEN
	hashes := make([][SHALEN]byte, cnt)
	for i := range cnt {
		copy(hashes[i][:], bys[i*SHALEN:(i+1)*SHALEN])
	}
	res.PieceSHA = hashes

	return res, nil
}
