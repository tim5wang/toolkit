package wav

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
)

type Head struct {
	ChunkID   [4]byte // 内容为"RIFF"
	ChunkSize uint32  // wav文件的字节数, 不包含ChunkID和ChunkSize这8个字节）
	Format    [4]byte // 内容为WAVE
}

type Fmt struct {
	Subchunk1ID   [4]byte // 内容为"fmt "
	Subchunk1Size uint32  // Fmt所占字节数，为16
	AudioFormat   uint16  // 存储音频的编码格式，pcm为1
	NumChannels   uint16  // 通道数, 单通道为1,双通道为2
	SampleRate    uint32  // 采样率，如8k, 44.1k等
	ByteRate      uint32  // 每秒存储的byte数，其值=SampleRate * NumChannels * BitsPerSample/8
	BlockAlign    uint16  // 块对齐大小，其值=NumChannels * BitsPerSample/8
	BitsPerSample uint16  // 每个采样点的bit数，一般为8,16,32等。
}

type Data struct {
	Subchunk2ID   [4]byte // 通常内容为"data", 如果不是, 应当跳过
	Subchunk2Size uint32  // 内容为接下来的正式的数据部分的字节数，其值=NumSamples * NumChannels * BitsPerSample/8
}

// WavHead wav的头通常认为是44 bytes,但是实际上经常并不是
type WavHead struct {
	Head
	Fmt
	Data
}

// WavHeaderWithExtra 适用于不确定wav header是 44byte的情况
type WavHeaderWithExtra struct {
	WavHead
	PcmOffset  int
	SampleNums int
}

func NewWavHeader(numChannels uint16, sampleRate uint32, bitsPerSample uint16, wavLenBytes uint32) *WavHead {
	head := &WavHead{
		Head: Head{
			// ChunkID:   []byte("RIFF"),
			ChunkSize: 36 + wavLenBytes,
			// Format:    []byte("WAVE"),
		},
		Fmt: Fmt{
			// Subchunk1ID:   []byte("fmt "),
			Subchunk1Size: 16,
			AudioFormat:   1,
			NumChannels:   numChannels,
			SampleRate:    sampleRate,
			ByteRate:      sampleRate * uint32(numChannels) * uint32(bitsPerSample) / 8,
			BlockAlign:    numChannels * bitsPerSample / 8,
			BitsPerSample: bitsPerSample,
		},
		Data: Data{
			// Subchunk2ID:   []byte("data"),
			Subchunk2Size: wavLenBytes,
		},
	}

	copy(head.ChunkID[:], "RIFF")
	copy(head.Format[:], "WAVE")
	copy(head.Subchunk1ID[:], "fmt ")
	copy(head.Subchunk2ID[:], "data")
	return head
}

func (wh *WavHead) Marshal() ([]byte, error) {
	buf := &bytes.Buffer{}
	err := binary.Write(buf, binary.LittleEndian, *wh)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func ReadWavHeader(wav []byte) (*WavHead, error) {
	reader := bytes.NewReader(wav)
	head := WavHead{}
	var err error
	// 不确定是大小端编码，就一种试一遍
	err = binary.Read(reader, binary.BigEndian, &head)
	if err != nil {
		return nil, err
	}
	if head.NumChannels > 200 {
		reader.Seek(0, 0)
		err = binary.Read(reader, binary.LittleEndian, &head)
		if err != nil {
			return nil, err
		}
	}
	return &head, nil
}

func ReadWavHeaderWithExtra(wav []byte) (*WavHeaderWithExtra, error) {
	reader := bytes.NewReader(wav)
	head := WavHead{}
	var err error
	// 不确定是大小端编码，就各试一遍
	var byteOrder binary.ByteOrder = binary.BigEndian
	err = binary.Read(reader, byteOrder, &head)
	if err != nil {
		return nil, err
	}
	if head.NumChannels > 200 {
		byteOrder = binary.LittleEndian
		_, err = reader.Seek(0, 0)
		if err != nil {
			return nil, err
		}
		err = binary.Read(reader, byteOrder, &head)
		if err != nil {
			return nil, err
		}
	}
	if head.NumChannels > 200 {
		return nil, errors.New("may not wav format")
	}
	extraHeader := WavHeaderWithExtra{
		WavHead:   head,
		PcmOffset: binary.Size(head),
	}
	var data = head.Data
	// 从前往后，直到 Sub chunkID == "data"
	for string(data.Subchunk2ID[:]) != "data" {
		extraHeader.PcmOffset += int(data.Subchunk2Size) + 8
		_, err = reader.Seek(int64(data.Subchunk2Size), io.SeekCurrent)
		if err != nil {
			return nil, err
		}
		err = binary.Read(reader, byteOrder, &data)
		if err != nil {
			return nil, err
		}
	}
	extraHeader.SampleNums = (len(wav) - extraHeader.PcmOffset) / (int(extraHeader.BitsPerSample) / 8)
	return &extraHeader, nil
}

func PcmToWav(numChannels uint16, sampleRate uint32, bitsPerSample uint16, pcm []byte) ([]byte, error) {
	head := NewWavHeader(numChannels, sampleRate, bitsPerSample, uint32(len(pcm)))
	hb, err := head.Marshal()
	if err != nil {
		return nil, err
	}
	res := make([]byte, len(hb)+len(pcm), len(hb)+len(pcm))
	copy(res[:len(hb)], hb[:])
	copy(res[len(hb):], pcm[:])
	return res, nil
}

func WavToPcm(wav []byte) ([]byte, *WavHeaderWithExtra, error) {
	head, err := ReadWavHeaderWithExtra(wav)
	if err != nil {
		return nil, nil, err
	}
	pcm := wav[head.PcmOffset:]
	return pcm, head, nil
}
