package launcher

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

// The exported managed DllMain enters NativeAOT even for thread detach.
// Windows invokes it under the loader lock, including after FLS cleanup.
// Only process attach actually does any work in this implementation. Keep
// that initialization, but return TRUE natively for all other notifications.
// The CRT entry point and NativeAOT's own TLS callbacks remain intact.
func configureProxyThreadNotifications(source []byte) ([]byte, error) {
	const sourceHash = "16d4f0cc8abfdc4065ddb34a982a5641866b9cca98ba8acc08407531ebfede27"
	if fmt.Sprintf("%x", sha256.Sum256(source)) != sourceHash {
		return nil, fmt.Errorf("代理模块版本不匹配，无法安全修正线程通知")
	}
	const codeOffset, codeRVA, unwindOffset = 23632, 0x6850, 1837344
	original, _ := hex.DecodeString("534883ec308bda488d4c2420e80fc40d0083fb017505e8d5eaffff488d4c2420e84bc50d00b8010000004883c4305bc3")
	if !bytes.Equal(source[codeOffset:codeOffset+48], original) || !bytes.Equal(source[unwindOffset:unwindOffset+8], []byte{1, 5, 2, 0, 5, 0x52, 1, 0x30}) {
		return nil, fmt.Errorf("代理模块线程入口结构不匹配")
	}
	// The early return precedes the stack allocation, so its unwind code is
	// correctly skipped. Managed calls retain 16-byte alignment, 32-byte shadow
	// space, and the reverse-P/Invoke frame at rsp+0x20.
	code := []byte{0x83, 0xfa, 1, 0x74, 6, 0xb8, 1, 0, 0, 0, 0xc3, 0x48, 0x83, 0xec, 0x38}
	frame := []byte{0x48, 0x8d, 0x4c, 0x24, 0x20}
	call := func(target uint32) {
		next := uint32(codeRVA + len(code) + 5)
		code = append(code, 0xe8)
		code = binary.LittleEndian.AppendUint32(code, target-next)
	}
	code = append(code, frame...)
	call(0xe2c70)
	call(0x5340)
	code = append(code, frame...)
	call(0xe2dc0)
	// DllMain returns BOOL: any nonzero value is TRUE.
	code = append(code, 0x83, 0xc8, 1, 0x48, 0x83, 0xc4, 0x38, 0xc3)
	if len(code) != 48 {
		return nil, fmt.Errorf("代理线程入口补丁长度错误")
	}
	result := bytes.Clone(source)
	copy(result[codeOffset:], code)
	// This unwind record is unique to DllMain in the pinned binary. Replace
	// push-rbx/allocate-48 with allocate-56, completed at offset 15.
	copy(result[unwindOffset:unwindOffset+8], []byte{1, 15, 1, 0, 15, 0x62, 0, 0})
	if err := patchProxyAllocationResult(result); err != nil {
		return nil, err
	}
	if err := patchProxyProtectionBase(result); err != nil {
		return nil, err
	}
	return result, nil
}
