package tcpstack

import (
	"bytes"
	"iptcp-nora-yu/pkg/tcpstack"
	"strings"
	"testing"
)

func TestRingBuffer_Write_Zero(t *testing.T) {
	rb := tcpstack.NewRecvBuf(64, 0) //start at 0 pos

	// check empty or full
	if !rb.IsEmpty() {
		t.Fatalf("expect IsEmpty is true but got false")
	}
	if rb.IsFull() {
		t.Fatalf("expect IsFull is false but got true")
	}
	if rb.WindowSize() != 0 {
		t.Fatalf("expect len 0 bytes but got %d. r.lbr=%d, r.nxt=%d", rb.WindowSize(), rb.LastByteRead(), rb.NextExpectedByte())
	}
	if rb.FreeSpace() != 64 {
		t.Fatalf("expect free 64 bytes but got %d. r.w=%d, r.nxt=%d", rb.FreeSpace(), rb.LastByteRead(), rb.NextExpectedByte())
	}

	// write 4 * 4 = 16 bytes
	n, err := rb.Write([]byte(strings.Repeat("abcd", 4)))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if n != 16 {
		t.Fatalf("expect write 16 bytes but got %d", n)
	}
	if rb.WindowSize() != 16 {
		t.Fatalf("expect len 16 bytes but got %d. r.lbr=%d, r.nxt=%d", rb.WindowSize(), rb.LastByteRead(), rb.NextExpectedByte())
	}
	if rb.FreeSpace() != 48 {
		t.Fatalf("expect free 48 bytes but got %d. r.w=%d, r.r=%d", rb.FreeSpace(), rb.LastByteRead(), rb.NextExpectedByte())
	}
	if !bytes.Equal(rb.Bytes(), []byte(strings.Repeat("abcd", 4))) {
		t.Fatalf("expect 4 abcd but got %s. r.w=%d, r.r=%d", rb.Bytes(), rb.LastByteRead(), rb.NextExpectedByte())
	}

	// check empty or full
	if rb.IsEmpty() {
		t.Fatalf("expect IsEmpty is false but got true")
	}
	if rb.IsFull() {
		t.Fatalf("expect IsFull is false but got true")
	}

	// write 48 bytes, should full
	n, err = rb.Write([]byte(strings.Repeat("abcd", 12)))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if n != 48 {
		t.Fatalf("expect write 48 bytes but got %d", n)
	}
	if rb.WindowSize() != 64 {
		t.Fatalf("expect len 64 bytes but got %d. r.lbr=%d, r.nxt=%d", rb.WindowSize(), rb.LastByteRead(), rb.NextExpectedByte())
	}
	if rb.FreeSpace() != 0 {
		t.Fatalf("expect free 0 bytes but got %d. r.lbr=%d, r.nxt=%d", rb.FreeSpace(), rb.LastByteRead(), rb.NextExpectedByte())
	}
	if rb.NextExpectedByte() != 65 {
		t.Fatalf("expect r.nxt=65 but got %d. r.lbr=%d", rb.NextExpectedByte(), rb.LastByteRead())
	}
	if !bytes.Equal(rb.Bytes(), []byte(strings.Repeat("abcd", 16))) {
		t.Fatalf("expect 16 abcd but got %s. r.lbr=%d, r.nxt=%d", rb.Bytes(), rb.LastByteRead(), rb.NextExpectedByte())
	}

	// check empty or full
	if rb.IsEmpty() {
		t.Fatalf("expect IsEmpty is false but got true")
	}
	if !rb.IsFull() {
		t.Fatalf("expect IsFull is true but got false")
	}

	// write more 4 bytes, should reject
	n, err = rb.Write([]byte(strings.Repeat("abcd", 1)))
	if err == nil {
		t.Fatalf("expect an error but got nil. n=%d, r.lbr=%d, r.nxt=%d", n, rb.LastByteRead(), rb.NextExpectedByte())
	}
	if n != 0 {
		t.Fatalf("expect write 0 bytes but got %d", n)
	}
	if rb.WindowSize() != 64 {
		t.Fatalf("expect len 64 bytes but got %d.  r.lbr=%d, r.nxt=%d", rb.WindowSize(), rb.LastByteRead(), rb.NextExpectedByte())
	}
	if rb.FreeSpace() != 0 {
		t.Fatalf("expect free 0 bytes but got %d.  r.lbr=%d, r.nxt=%d", rb.FreeSpace(), rb.LastByteRead(), rb.NextExpectedByte())
	}

	// check empty or full
	if rb.IsEmpty() {
		t.Fatalf("expect IsEmpty is false but got true")
	}
	if !rb.IsFull() {
		t.Fatalf("expect IsFull is true but got false")
	}

}

func TestRingBuffer_Write_Random_Within_Range(t *testing.T) {
	rb := tcpstack.NewRecvBuf(64, 29) //start at 0 pos

	//check ptr
	if rb.LastByteRead() != 29 {
		t.Fatalf("expect lbr start point at 29 but got %d", rb.LastByteRead())
	}
	if rb.NextExpectedByte() != 30 {
		t.Fatalf("expect nxt start point at 30 but got %d", rb.NextExpectedByte())
	}
	// check empty or full
	if !rb.IsEmpty() {
		t.Fatalf("expect IsEmpty is true but got false")
	}
	if rb.IsFull() {
		t.Fatalf("expect IsFull is false but got true")
	}
	if rb.WindowSize() != 0 {
		t.Fatalf("expect len 0 bytes but got %d. r.lbr=%d, r.nxt=%d", rb.WindowSize(), rb.LastByteRead(), rb.NextExpectedByte())
	}
	if rb.FreeSpace() != 64 {
		t.Fatalf("expect free 64 bytes but got %d. r.w=%d, r.nxt=%d", rb.FreeSpace(), rb.LastByteRead(), rb.NextExpectedByte())
	}

	// write 4 * 4 = 16 bytes
	n, err := rb.Write([]byte(strings.Repeat("abcd", 4)))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if n != 16 {
		t.Fatalf("expect write 16 bytes but got %d", n)
	}
	if rb.WindowSize() != 16 {
		t.Fatalf("expect len 16 bytes but got %d. r.lbr=%d, r.nxt=%d", rb.WindowSize(), rb.LastByteRead(), rb.NextExpectedByte())
	}
	if rb.FreeSpace() != 48 {
		t.Fatalf("expect free 48 bytes but got %d. r.w=%d, r.r=%d", rb.FreeSpace(), rb.LastByteRead(), rb.NextExpectedByte())
	}
	if !bytes.Equal(rb.Bytes(), []byte(strings.Repeat("abcd", 4))) {
		t.Fatalf("expect 4 abcd but got %s. r.w=%d, r.r=%d", rb.Bytes(), rb.LastByteRead(), rb.NextExpectedByte())
	}

	// check empty or full
	if rb.IsEmpty() {
		t.Fatalf("expect IsEmpty is false but got true")
	}
	if rb.IsFull() {
		t.Fatalf("expect IsFull is false but got true")
	}

	// write 48 bytes, should full
	n, err = rb.Write([]byte(strings.Repeat("abcd", 12)))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if n != 48 {
		t.Fatalf("expect write 48 bytes but got %d", n)
	}
	if rb.WindowSize() != 64 {
		t.Fatalf("expect len 64 bytes but got %d. r.lbr=%d, r.nxt=%d", rb.WindowSize(), rb.LastByteRead(), rb.NextExpectedByte())
	}
	if rb.FreeSpace() != 0 {
		t.Fatalf("expect free 0 bytes but got %d. r.lbr=%d, r.nxt=%d", rb.FreeSpace(), rb.LastByteRead(), rb.NextExpectedByte())
	}
	if rb.NextExpectedByte() != 29+64+1 {
		t.Fatalf("expect r.nxt=65 but got %d. r.lbr=%d", rb.NextExpectedByte(), rb.LastByteRead())
	}
	if !bytes.Equal(rb.Bytes(), []byte(strings.Repeat("abcd", 16))) {
		t.Fatalf("expect 16 abcd but got %s. r.lbr=%d, r.nxt=%d", rb.Bytes(), rb.LastByteRead(), rb.NextExpectedByte())
	}

	// check empty or full
	if rb.IsEmpty() {
		t.Fatalf("expect IsEmpty is false but got true")
	}
	if !rb.IsFull() {
		t.Fatalf("expect IsFull is true but got false")
	}

	// write more 4 bytes, should reject
	n, err = rb.Write([]byte(strings.Repeat("abcd", 1)))
	if err == nil {
		t.Fatalf("expect an error but got nil. n=%d, r.lbr=%d, r.nxt=%d", n, rb.LastByteRead(), rb.NextExpectedByte())
	}
	if n != 0 {
		t.Fatalf("expect write 0 bytes but got %d", n)
	}
	if rb.WindowSize() != 64 {
		t.Fatalf("expect len 64 bytes but got %d.  r.lbr=%d, r.nxt=%d", rb.WindowSize(), rb.LastByteRead(), rb.NextExpectedByte())
	}
	if rb.FreeSpace() != 0 {
		t.Fatalf("expect free 0 bytes but got %d.  r.lbr=%d, r.nxt=%d", rb.FreeSpace(), rb.LastByteRead(), rb.NextExpectedByte())
	}

	// check empty or full
	if rb.IsEmpty() {
		t.Fatalf("expect IsEmpty is false but got true")
	}
	if !rb.IsFull() {
		t.Fatalf("expect IsFull is true but got false")
	}
}

func TestRingBuffer_Write_Random_Out_Range(t *testing.T) {
	rb := tcpstack.NewRecvBuf(64, 2031) //start at 0 pos

	//check ptr
	if rb.LastByteRead() != 2031 {
		t.Fatalf("expect lbr start point at 29 but got %d", rb.LastByteRead())
	}
	if rb.NextExpectedByte() != 2031+1 {
		t.Fatalf("expect nxt start point at 30 but got %d", rb.NextExpectedByte())
	}
	// check empty or full
	if !rb.IsEmpty() {
		t.Fatalf("expect IsEmpty is true but got false")
	}
	if rb.IsFull() {
		t.Fatalf("expect IsFull is false but got true")
	}
	if rb.WindowSize() != 0 {
		t.Fatalf("expect len 0 bytes but got %d. r.lbr=%d, r.nxt=%d", rb.WindowSize(), rb.LastByteRead(), rb.NextExpectedByte())
	}
	if rb.FreeSpace() != 64 {
		t.Fatalf("expect free 64 bytes but got %d. r.w=%d, r.nxt=%d", rb.FreeSpace(), rb.LastByteRead(), rb.NextExpectedByte())
	}

	// write 4 * 4 = 16 bytes
	n, err := rb.Write([]byte(strings.Repeat("abcd", 4)))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if n != 16 {
		t.Fatalf("expect write 16 bytes but got %d", n)
	}
	if rb.WindowSize() != 16 {
		t.Fatalf("expect len 16 bytes but got %d. r.lbr=%d, r.nxt=%d", rb.WindowSize(), rb.LastByteRead(), rb.NextExpectedByte())
	}
	if rb.FreeSpace() != 48 {
		t.Fatalf("expect free 48 bytes but got %d. r.w=%d, r.r=%d", rb.FreeSpace(), rb.LastByteRead(), rb.NextExpectedByte())
	}
	if !bytes.Equal(rb.Bytes(), []byte(strings.Repeat("abcd", 4))) {
		t.Fatalf("expect 4 abcd but got %s. r.w=%d, r.r=%d", rb.Bytes(), rb.LastByteRead(), rb.NextExpectedByte())
	}

	// check empty or full
	if rb.IsEmpty() {
		t.Fatalf("expect IsEmpty is false but got true")
	}
	if rb.IsFull() {
		t.Fatalf("expect IsFull is false but got true")
	}

	// write 48 bytes, should full
	n, err = rb.Write([]byte(strings.Repeat("abcd", 12)))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if n != 48 {
		t.Fatalf("expect write 48 bytes but got %d", n)
	}
	if rb.WindowSize() != 64 {
		t.Fatalf("expect len 64 bytes but got %d. r.lbr=%d, r.nxt=%d", rb.WindowSize(), rb.LastByteRead(), rb.NextExpectedByte())
	}
	if rb.FreeSpace() != 0 {
		t.Fatalf("expect free 0 bytes but got %d. r.lbr=%d, r.nxt=%d", rb.FreeSpace(), rb.LastByteRead(), rb.NextExpectedByte())
	}
	if rb.NextExpectedByte() != 2031+64+1 {
		t.Fatalf("expect r.nxt=65 but got %d. r.lbr=%d", rb.NextExpectedByte(), rb.LastByteRead())
	}
	if !bytes.Equal(rb.Bytes(), []byte(strings.Repeat("abcd", 16))) {
		t.Fatalf("expect 16 abcd but got %s. r.lbr=%d, r.nxt=%d", rb.Bytes(), rb.LastByteRead(), rb.NextExpectedByte())
	}

	// check empty or full
	if rb.IsEmpty() {
		t.Fatalf("expect IsEmpty is false but got true")
	}
	if !rb.IsFull() {
		t.Fatalf("expect IsFull is true but got false")
	}

	// write more 4 bytes, should reject
	n, err = rb.Write([]byte(strings.Repeat("abcd", 1)))
	if err == nil {
		t.Fatalf("expect an error but got nil. n=%d, r.lbr=%d, r.nxt=%d", n, rb.LastByteRead(), rb.NextExpectedByte())
	}
	if n != 0 {
		t.Fatalf("expect write 0 bytes but got %d", n)
	}
	if rb.WindowSize() != 64 {
		t.Fatalf("expect len 64 bytes but got %d.  r.lbr=%d, r.nxt=%d", rb.WindowSize(), rb.LastByteRead(), rb.NextExpectedByte())
	}
	if rb.FreeSpace() != 0 {
		t.Fatalf("expect free 0 bytes but got %d.  r.lbr=%d, r.nxt=%d", rb.FreeSpace(), rb.LastByteRead(), rb.NextExpectedByte())
	}

	// check empty or full
	if rb.IsEmpty() {
		t.Fatalf("expect IsEmpty is false but got true")
	}
	if !rb.IsFull() {
		t.Fatalf("expect IsFull is true but got false")
	}
}

func TestRingBuffer_Read(t *testing.T) {
	start := uint32(2031)
	rb := tcpstack.NewRecvBuf(64, start)

	//check ptr
	if rb.LastByteRead() != start {
		t.Fatalf("expect lbr start point at 2031 but got %d", rb.LastByteRead())
	}
	if rb.NextExpectedByte() != start+1 {
		t.Fatalf("expect nxt start point at 2032 but got %d", rb.NextExpectedByte())
	}
	// check empty or full
	if !rb.IsEmpty() {
		t.Fatalf("expect IsEmpty is true but got false")
	}
	if rb.IsFull() {
		t.Fatalf("expect IsFull is false but got true")
	}
	if rb.WindowSize() != 0 {
		t.Fatalf("expect len 0 bytes but got %d. r.lbr=%d, r.nxt=%d", rb.WindowSize(), rb.LastByteRead(), rb.NextExpectedByte())
	}
	if rb.FreeSpace() != 64 {
		t.Fatalf("expect free 64 bytes but got %d. r.w=%d, r.nxt=%d", rb.FreeSpace(), rb.LastByteRead(), rb.NextExpectedByte())
	}

	// read empty will timeut
	buf := make([]byte, 1024)
	// n, err := rb.Read(buf)
	// if err == nil {
	// 	t.Fatalf("expect an error but got nil")
	// }
	// if n != 0 {
	// 	t.Fatalf("expect read 0 bytes but got %d", n)
	// }
	// if rb.WindowSize() != 0 {
	// 	t.Fatalf("expect len 0 bytes but got %d. r.lbr=%d, r.nxt=%d", rb.WindowSize(), rb.LastByteRead(), rb.NextExpectedByte())
	// }
	// if rb.FreeSpace() != 64 {
	// 	t.Fatalf("expect free 64 bytes but got %d. r.w=%d, r.nxt=%d", rb.FreeSpace(), rb.LastByteRead(), rb.NextExpectedByte())
	// }
	// if rb.LastByteRead() != start {
	// 	t.Fatalf("expect r.lbr = 2031 but got %d", rb.LastByteRead())
	// }

	// write 16 bytes to read and read all
	rb.Write([]byte(strings.Repeat("abcd", 4)))
	n, err := rb.Read(buf)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if n != 16 {
		t.Fatalf("expect read 16 bytes but got %d", n)
	}
	if rb.WindowSize() != 0 {
		t.Fatalf("expect len 0 bytes but got %d. r.lbr=%d, r.nxt=%d", rb.WindowSize(), rb.LastByteRead(), rb.NextExpectedByte())
	}
	if rb.FreeSpace() != 64 {
		t.Fatalf("expect free 64 bytes but got %d. r.w=%d, r.nxt=%d", rb.FreeSpace(), rb.LastByteRead(), rb.NextExpectedByte())
	}
	if rb.LastByteRead() != start+16 {
		t.Fatalf("expect r.lbr = %d but got %d", start+16, rb.LastByteRead())
	}

	//read partial
	rb = tcpstack.NewRecvBuf(64, start)
	buf = make([]byte, 4)
	rb.Write([]byte(strings.Repeat("abcd", 4)))
	n, err = rb.Read(buf)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if n != 4 {
		t.Fatalf("expect read 4 bytes but got %d", n)
	}
	if rb.WindowSize() != 12 {
		t.Fatalf("expect len 12 bytes but got %d. r.lbr=%d, r.nxt=%d", rb.WindowSize(), rb.LastByteRead(), rb.NextExpectedByte())
	}
	if rb.FreeSpace() != 52 {
		t.Fatalf("expect free 52 bytes but got %d. r.w=%d, r.nxt=%d", rb.FreeSpace(), rb.LastByteRead(), rb.NextExpectedByte())
	}
	if rb.LastByteRead() != start+4 {
		t.Fatalf("expect r.lbr = %d but got %d", start+4, rb.LastByteRead())
	}
}
