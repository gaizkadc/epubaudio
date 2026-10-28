package tts

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	trustedClientToken = "6A5AA1D4EAFF4E9FB37E23D68491D6F4"
	winEpoch           = 11644473600
	sToNs              = 1e9

	chromiumFullVersion = "143.0.3650.75"
	secMsGecVersion     = "2-" + chromiumFullVersion

	cs64Factor      = 0x82893933
	cs64Adjustment0 = 0xF2F90B1A
	cs64Adjustment1 = 0x0B1A0000

	sha256Adjustment      = 0xC9DC06A7
	sha256Sigma0Adjust    = 0x059C0B80
	sha256Sigma1Adjust    = 0x000B92C0
	mask32                = 0xFFFFFFFF
)

var sha256Initial = [8]uint32{
	0x6A09E667, 0xBB67AE85, 0x3C6EF372, 0xA54FF53A,
	0x510E527F, 0x9B05688C, 0x1F83D9AB, 0x5BE0CD19,
}

var sha256State = [8]uint32{
	0xFD9FBEA2, 0x91243CE7, 0xBE499094, 0xCD2E1E0A,
	0x6C006F49, 0x04BD35AB, 0xFEB0A7E4, 0x03EFFBD2,
}

var sha256Schedule = [14]uint32{
	0xEB24BF9E, 0x1558691F, 0xA0C09EE2, 0x0CFA32BA,
	0xA622EE8C, 0xD1DD9491, 0xC0DD25A0, 0xF9E70AB6,
	0xF0C31573, 0x7058CE4E, 0x7188E606, 0x00E5AB33,
	0xD258B060, 0x72766564,
}

var sha256RoundConstants = [64]uint32{
	0x428A2F98, 0x71374491, 0xB5C0FBCF, 0xE9B5DBA5, 0x3956C25B, 0x59F111F1, 0x923F82A4, 0xAB1C5ED5,
	0xD807AA98, 0x12835B01, 0x243185BE, 0x550C7DC3, 0x72BE5D74, 0x80DEB1FE, 0x9BDC06A7, 0xC19BF174,
	0xE49B69C1, 0xEFBE4786, 0x0FC19DC6, 0x240CA1CC, 0x2DE92C6F, 0x4A7484AA, 0x5CB0A9DC, 0x76F988DA,
	0x983E5152, 0xA831C66D, 0xB00327C8, 0xBF597FC7, 0xC6E00BF3, 0xD5A79147, 0x06CA6351, 0x14292967,
	0x27B70A85, 0x2E1B2138, 0x4D2C6DFC, 0x53380D13, 0x650A7354, 0x766A0ABB, 0x81C2C92E, 0x92722C85,
	0xA2BFE8A1, 0xA81A664B, 0xC24B8B70, 0xC76C51A3, 0xD192E819, 0xD6990624, 0xF40E3585, 0x106AA070,
	0x19A4C116, 0x1E376C08, 0x2748774C, 0x34B0BCB5, 0x391C0CB3, 0x4ED8AA4A, 0x5B9CCA4F, 0x682E6FF3,
	0x748F82EE, 0x78A5636F, 0x84C87814, 0x8CC70208, 0x90BEFFFA, 0xA4506CEB, 0xBEF9A3F7, 0xC67178F2,
}

var sha256BlockConstants [64]uint32

func init() {
	copy(sha256BlockConstants[:14], sha256RoundConstants[:14])
	sha256BlockConstants[14] = sha256Adjustment
	copy(sha256BlockConstants[15:], sha256RoundConstants[15:])
}

type cs64Coeff struct {
	coeff  uint32
	offset uint32
}

var cs64Coefficients = [14]cs64Coeff{
	{0xE82E91B1, 0xB32700D0},
	{0x44DE24C6, 0x88DFF51D},
	{0x3BEB65BC, 0x29514038},
	{0xC89C5933, 0xB328CBDF},
	{0x8D1EC211, 0xD405B4D5},
	{0x314EF5D5, 0x26E55F3C},
	{0xE92491B1, 0x607801C6},
	{0xF0A5B9D5, 0x82B2C1FB},
	{0x06064CF3, 0xFA51A3DA},
	{0xDA79FA60, 0x207CC464},
	{0xBED20560, 0x3C3C74F3},
	{0x89F9C9F8, 0xD2D14B61},
	{0x35FA38C6, 0xBB04A2C1},
	{0xCB49E3C6, 0xC5626167},
}

func rol16(v uint32) uint32 {
	return ((v << 16) | (v >> 16)) & mask32
}

func wordsLE4Even(data []byte) []uint32 {
	rem := len(data) % 4
	var padded []byte
	if rem != 0 {
		padded = make([]byte, len(data)+4-rem)
		copy(padded, data)
	} else {
		padded = data
	}
	if len(padded) == 0 {
		return nil
	}
	numWords := len(padded) / 4
	words := make([]uint32, numWords)
	for i := 0; i < numWords; i++ {
		words[i] = binary.LittleEndian.Uint32(padded[i*4 : (i+1)*4])
	}
	if len(words)%2 != 0 {
		words = append(words, 0)
	}
	return words
}

func sigma0(v uint32) uint32 {
	return ((v >> 7) ^ (v << 25) ^ (v >> 18) ^ (v << 14) ^ (v >> 3)) & mask32
}

func sigma1(v uint32) uint32 {
	return ((v >> 17) ^ (v << 15) ^ (v >> 19) ^ (v << 13) ^ (v >> 10)) & mask32
}

func sha256Rounds(state [8]uint32, words []uint32, firstRound int) [8]uint32 {
	a, b, c, d, e, f, g, h := state[0], state[1], state[2], state[3], state[4], state[5], state[6], state[7]
	constants := sha256RoundConstants
	if firstRound == 14 {
		constants = sha256BlockConstants
	}

	aligned := 64
	val := ((firstRound + 7) / 8) * 8
	if val < aligned {
		aligned = val
	}

	for i := firstRound; i < aligned; i++ {
		rotE := uint64(e) * 0x100000001
		rotA := uint64(a) * 0x100000001
		t1 := uint32((uint64(h) + ((rotE >> 6) ^ (rotE >> 11) ^ (rotE >> 25)) + uint64(g^(e&(f^g))) + uint64(constants[i]) + uint64(words[i])) & mask32)
		t2 := uint32((((rotA >> 2) ^ (rotA >> 13) ^ (rotA >> 22)) + uint64((a&b)|(c&(a|b)))) & mask32)
		a, b, c, d, e, f, g, h = (t1+t2)&mask32, a, b, c, (d+t1)&mask32, e, f, g
	}

	for i := aligned; i < 64; i += 8 {
		rotE := uint64(e) * 0x100000001
		rotA := uint64(a) * 0x100000001
		t1 := uint32((uint64(h) + ((rotE >> 6) ^ (rotE >> 11) ^ (rotE >> 25)) + uint64(g^(e&(f^g))) + uint64(constants[i]) + uint64(words[i])) & mask32)
		d = (d + t1) & mask32
		h = uint32((uint64(t1) + ((rotA >> 2) ^ (rotA >> 13) ^ (rotA >> 22)) + uint64((a&b)|(c&(a|b)))) & mask32)

		rotE = uint64(d) * 0x100000001
		rotA = uint64(h) * 0x100000001
		t1 = uint32((uint64(g) + ((rotE >> 6) ^ (rotE >> 11) ^ (rotE >> 25)) + uint64(f^(d&(e^f))) + uint64(constants[i+1]) + uint64(words[i+1])) & mask32)
		c = (c + t1) & mask32
		g = uint32((uint64(t1) + ((rotA >> 2) ^ (rotA >> 13) ^ (rotA >> 22)) + uint64((h&a)|(b&(h|a)))) & mask32)

		rotE = uint64(c) * 0x100000001
		rotA = uint64(g) * 0x100000001
		t1 = uint32((uint64(f) + ((rotE >> 6) ^ (rotE >> 11) ^ (rotE >> 25)) + uint64(e^(c&(d^e))) + uint64(constants[i+2]) + uint64(words[i+2])) & mask32)
		b = (b + t1) & mask32
		f = uint32((uint64(t1) + ((rotA >> 2) ^ (rotA >> 13) ^ (rotA >> 22)) + uint64((g&h)|(a&(g|h)))) & mask32)

		rotE = uint64(b) * 0x100000001
		rotA = uint64(f) * 0x100000001
		t1 = uint32((uint64(e) + ((rotE >> 6) ^ (rotE >> 11) ^ (rotE >> 25)) + uint64(d^(b&(c^d))) + uint64(constants[i+3]) + uint64(words[i+3])) & mask32)
		a = (a + t1) & mask32
		e = uint32((uint64(t1) + ((rotA >> 2) ^ (rotA >> 13) ^ (rotA >> 22)) + uint64((f&g)|(h&(f|g)))) & mask32)

		rotE = uint64(a) * 0x100000001
		rotA = uint64(e) * 0x100000001
		t1 = uint32((uint64(d) + ((rotE >> 6) ^ (rotE >> 11) ^ (rotE >> 25)) + uint64(c^(a&(b^c))) + uint64(constants[i+4]) + uint64(words[i+4])) & mask32)
		h = (h + t1) & mask32
		d = uint32((uint64(t1) + ((rotA >> 2) ^ (rotA >> 13) ^ (rotA >> 22)) + uint64((e&f)|(g&(e|f)))) & mask32)

		rotE = uint64(h) * 0x100000001
		rotA = uint64(d) * 0x100000001
		t1 = uint32((uint64(c) + ((rotE >> 6) ^ (rotE >> 11) ^ (rotE >> 25)) + uint64(b^(h&(a^b))) + uint64(constants[i+5]) + uint64(words[i+5])) & mask32)
		g = (g + t1) & mask32
		c = uint32((uint64(t1) + ((rotA >> 2) ^ (rotA >> 13) ^ (rotA >> 22)) + uint64((d&e)|(f&(d|e)))) & mask32)

		rotE = uint64(g) * 0x100000001
		rotA = uint64(c) * 0x100000001
		t1 = uint32((uint64(b) + ((rotE >> 6) ^ (rotE >> 11) ^ (rotE >> 25)) + uint64(a^(g&(h^a))) + uint64(constants[i+6]) + uint64(words[i+6])) & mask32)
		f = (f + t1) & mask32
		b = uint32((uint64(t1) + ((rotA >> 2) ^ (rotA >> 13) ^ (rotA >> 22)) + uint64((c&d)|(e&(c|d)))) & mask32)

		rotE = uint64(f) * 0x100000001
		rotA = uint64(b) * 0x100000001
		t1 = uint32((uint64(a) + ((rotE >> 6) ^ (rotE >> 11) ^ (rotE >> 25)) + uint64(h^(f&(g^h))) + uint64(constants[i+7]) + uint64(words[i+7])) & mask32)
		e = (e + t1) & mask32
		a = uint32((uint64(t1) + ((rotA >> 2) ^ (rotA >> 13) ^ (rotA >> 22)) + uint64((b&c)|(d&(b|c)))) & mask32)
	}

	return [8]uint32{a, b, c, d, e, f, g, h}
}

func expandSha256Schedule(words *[]uint32, firstWord int) {
	w := *words
	for i := firstWord; i < 64; i++ {
		x, y := w[i-2], w[i-15]
		s1 := ((x >> 17) ^ (x << 15) ^ (x >> 19) ^ (x << 13) ^ (x >> 10))
		s0 := ((y >> 7) ^ (y << 25) ^ (y >> 18) ^ (y << 14) ^ (y >> 3))
		w = append(w, (s1+w[i-7]+s0+w[i-16])&mask32)
	}
	*words = w
}

func sha256Init(data []byte) [8]uint32 {
	words := make([]uint32, 14, 64)
	w14 := (uint32(data[0]) << 16) | (uint32(data[1]) << 8) | uint32(data[2])
	w15 := binary.BigEndian.Uint32(data[3:7])
	words = append(words, w14, w15)

	for i := 16; i < 30; i++ {
		val := sha256Schedule[i-16]
		if i-2 >= 14 {
			if i == 16 {
				val += sigma1(words[i-2]) ^ sha256Sigma1Adjust
			} else {
				val += sigma1(words[i-2])
			}
		}
		if i-7 >= 14 {
			val += words[i-7]
		}
		if i-15 >= 14 {
			if i == 29 {
				val += sigma0(words[i-15]) ^ sha256Sigma0Adjust
			} else {
				val += sigma0(words[i-15])
			}
		}
		words = append(words, val&mask32)
	}

	w30 := (sigma1(words[28]) + words[23] + sigma0(words[15]) + words[14] + sha256Adjustment - sha256RoundConstants[14]) & mask32
	words = append(words, w30)
	expandSha256Schedule(&words, 31)

	working := sha256Rounds(sha256State, words, 14)
	var res [8]uint32
	for i := 0; i < 8; i++ {
		res[i] = (sha256Initial[i] + working[i]) & mask32
	}
	return res
}

func sha256Digest(data []byte) []byte {
	length := 57 + len(data)
	rem := (55 - length) % 64
	if rem < 0 {
		rem += 64
	}
	padded := make([]byte, 0, len(data)-7+1+rem+8)
	padded = append(padded, data[7:]...)
	padded = append(padded, 0x80)
	padded = append(padded, make([]byte, rem)...)
	var bitLenBuf [8]byte
	binary.BigEndian.PutUint64(bitLenBuf[:], uint64(length*8))
	padded = append(padded, bitLenBuf[:]...)

	state := sha256Init(data[:7])
	for offset := 0; offset < len(padded); offset += 64 {
		block := padded[offset : offset+64]
		words := make([]uint32, 16, 64)
		for j := 0; j < 16; j++ {
			words[j] = binary.BigEndian.Uint32(block[j*4 : (j+1)*4])
		}
		expandSha256Schedule(&words, 16)
		working := sha256Rounds(state, words, 0)
		for j := 0; j < 8; j++ {
			state[j] = (state[j] + working[j]) & mask32
		}
	}

	out := make([]byte, 32)
	for i := 0; i < 8; i++ {
		binary.BigEndian.PutUint32(out[i*4:(i+1)*4], state[i])
	}
	return out
}

func cs64Digest(data []byte, digest []byte) (uint32, uint32) {
	keyA := binary.LittleEndian.Uint32(digest[:4]) | 1
	keyB := binary.LittleEndian.Uint32(digest[4:8]) | 1

	factors := [2]uint32{
		(keyA * cs64Factor) & mask32,
		(keyB * cs64Factor) & mask32,
	}

	terms := make([]uint32, 0, 14+len(data)/2)
	offsets := make([]uint32, 0, 14+len(data)/2)

	for i, c := range cs64Coefficients {
		terms = append(terms, (c.coeff*factors[i%2])&mask32)
		offsets = append(offsets, c.offset)
	}

	prepended := make([]byte, len(data)+1)
	copy(prepended[1:], data)
	words := wordsLE4Even(prepended)

	for i, word := range words {
		var k uint32
		if i%2 == 0 {
			k = keyA
		} else {
			k = keyB
		}
		term := (word * k) & mask32

		var mult uint32
		if i%2 == 0 {
			mult = 0xEF0569FB
		} else {
			mult = 0xC31713DB
		}
		offset := (rol16(word) * mult) & mask32

		if i == 0 {
			term = (term + cs64Adjustment0*factors[0]) & mask32
			offset = (offset + cs64Adjustment1) & mask32
		}
		terms = append(terms, term)
		offsets = append(offsets, offset)
	}

	var swapAcc, swapLast, state, acc uint32
	for i := 0; i < len(terms); i += 2 {
		x, y := terms[i], terms[i+1]
		t := rol16((offsets[i] + x) & mask32)
		q := (t * 0x76F50F34) & mask32
		swapAcc = (swapAcc + rol16(q)*0xC3EFEA97 + q) & mask32

		t = rol16((offsets[i+1] + y) & mask32)
		q = (t * 0x1881CE3C) & mask32
		swapLast = (rol16(q)*0x35BD1EC9 + q) & mask32
		swapAcc = (swapAcc + swapLast) & mask32

		state = (state*keyA + x) & mask32
		state = (rol16(state) * 0xCF98B111) & mask32
		state = (rol16(state) * 0x87085B9F) & mask32
		state = (rol16(state) * 0x12CEB96D) & mask32
		state = (rol16(state) * 0x257E1D83) & mask32
		acc = (acc + state) & mask32

		t = (state*keyB + y) & mask32
		t = (rol16(t) * 0xA27416F5) & mask32
		t = (rol16(t) * 0xD38396FF) & mask32
		state = (rol16((rol16(t)*0x7C932B89)&mask32) * 0xBFA49F69) & mask32
		acc = (acc + state) & mask32
	}

	return swapLast ^ state, swapAcc ^ acc
}

// GenerateSecMSGecV2 produces the Sec-MS-GEC v2 token required by Microsoft Edge's Read Aloud service.
func GenerateSecMSGecV2() string {
	now := float64(time.Now().UTC().Unix())
	ticks := now + winEpoch
	ticks -= float64(int64(ticks) % 300)
	ticks *= sToNs / 100

	strToHash := fmt.Sprintf("%.0f%s", ticks, trustedClientToken)
	digest := sha256Digest([]byte(strToHash))
	w1, w2 := cs64Digest([]byte(strToHash), digest)

	var cs64Buf [8]byte
	binary.LittleEndian.PutUint32(cs64Buf[:4], w1)
	binary.LittleEndian.PutUint32(cs64Buf[4:8], w2)

	return fmt.Sprintf("%s-%s", strings.ToUpper(hex.EncodeToString(cs64Buf[:])), strings.ToUpper(hex.EncodeToString(digest)))
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return strings.ToUpper(hex.EncodeToString(b))
}

