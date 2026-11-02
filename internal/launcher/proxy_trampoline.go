package launcher

import (
	"bytes"
	"fmt"
)

// Called only on the private copy of the hash-pinned proxy binary in
// configureProxyThreadNotifications. Both successful near-allocation paths
// returned the requested address, not VirtualAlloc's result. Windows can round
// an allocation address, so writing a trampoline through the requested pointer
// can run off the end of the committed allocation.
func patchProxyAllocationResult(module []byte) error {
	patches := []struct {
		offset        int
		before, after []byte
	}{
		// RVA f1f1: mov rax,rsi (requested) -> mov rax,r14 (VirtualAlloc result).
		{58865, []byte{0x48, 0x8b, 0xc6}, []byte{0x49, 0x8b, 0xc6}},
		// RVA f368: mov rax,[rbp+60h] (requested) -> mov rax,rdi; nop (result).
		{59240, []byte{0x48, 0x8b, 0x45, 0x60}, []byte{0x48, 0x8b, 0xc7, 0x90}},
	}
	// Check all sites before editing any; retain original layout and unwind data.
	for _, patch := range patches {
		if patch.offset+len(patch.before) > len(module) || !bytes.Equal(module[patch.offset:patch.offset+len(patch.before)], patch.before) {
			return fmt.Errorf("代理模块跳板分配结构不匹配，无法安全修正")
		}
	}
	for _, patch := range patches {
		copy(module[patch.offset:], patch.after)
	}
	return nil
}

// VirtualQuery describes an entire region. Pair its RegionSize with its
// BaseAddress, not the queried function address: the latter can extend the
// VirtualProtect range into an uncommitted page and leave code read-only.
func patchProxyProtectionBase(module []byte) error {
	const offset = 59332 // RVA f3c4; RAX points to the VirtualQuery result.
	if len(module) < offset+3 || !bytes.Equal(module[offset:offset+3], []byte{0x48, 0x8b, 0xce}) {
		return fmt.Errorf("代理模块内存保护结构不匹配，无法安全修正")
	}
	copy(module[offset:], []byte{0x48, 0x8b, 0x08}) // mov rcx,[rax]
	return nil
}
