package launcher

import (
	"bytes"
	"crypto/sha256"
	"fmt"
)

// This supported binary has one TerminateThread caller, in its worker's
// shutdown path. A forced exit bypasses FLS/TLS destructors and can leave an
// attached NativeAOT thread in the proxy's thread list after its TLS is freed.
// Keep the worker exit, but use ExitThread(0), allowing normal cleanup.
func configureFeatureThreadExit(module []byte) ([]byte, error) {
	const sourceHash = "a55efc24a6ada07a9d24889c97182bbb1eb840297e24252e410b02096a8b8b1a"
	if fmt.Sprintf("%x", sha256.Sum256(module)) != sourceHash {
		return nil, fmt.Errorf("游戏功能模块版本不匹配，无法安全修正线程退出")
	}
	const importNameOffset = 560856
	const argumentOffset = 33191
	if !bytes.Equal(module[importNameOffset:importNameOffset+16], []byte("TerminateThread\x00")) ||
		!bytes.Equal(module[argumentOffset:argumentOffset+3], []byte{0x48, 0x89, 0xc1}) {
		return nil, fmt.Errorf("游戏功能模块的线程退出结构不匹配")
	}
	result := bytes.Clone(module)
	copy(result[importNameOffset:importNameOffset+16], []byte("ExitThread\x00\x00\x00\x00\x00\x00"))
	// mov rcx, rax (current thread handle) -> xor ecx, ecx; nop (exit code 0).
	copy(result[argumentOffset:argumentOffset+3], []byte{0x31, 0xc9, 0x90})
	return result, nil
}
