//go:build windows

package main

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows/registry"
	"poem/internal/helperproto"
)

// applyRegistry writes values in the prefix's registry. Wine's registry API is
// complete enough for this; reg.exe would need a separate Wine process.
func applyRegistry(values []helperproto.RegValue) error {
	for _, v := range values {
		var root registry.Key
		path := v.Key
		switch {
		case strings.HasPrefix(path, `HKCU\`):
			root, path = registry.CURRENT_USER, path[5:]
		case strings.HasPrefix(path, `HKLM\`):
			root, path = registry.LOCAL_MACHINE, path[5:]
		default:
			return fmt.Errorf("unsupported registry root: %s", v.Key)
		}
		if v.Kind == helperproto.RegDelete {
			k, err := registry.OpenKey(root, path, registry.SET_VALUE)
			if err == registry.ErrNotExist {
				continue
			}
			if err != nil {
				return fmt.Errorf("%s: %w", v.Key, err)
			}
			err = k.DeleteValue(v.Name)
			k.Close()
			if err != nil && err != registry.ErrNotExist {
				return fmt.Errorf(`%s\%s: %w`, v.Key, v.Name, err)
			}
			continue
		}
		k, _, err := registry.CreateKey(root, path, registry.QUERY_VALUE|registry.SET_VALUE)
		if err != nil {
			return fmt.Errorf("%s: %w", v.Key, err)
		}
		switch v.Kind {
		case helperproto.RegPrepend:
			list, _, _ := k.GetStringsValue(v.Name) // missing or not REG_MULTI_SZ: start empty
			err = k.SetStringsValue(v.Name, helperproto.PrependEntry(list, v.String))
		case helperproto.RegString:
			err = k.SetStringValue(v.Name, v.String)
		case helperproto.RegDWord:
			err = k.SetDWordValue(v.Name, v.DWord)
		case helperproto.RegBinary:
			err = k.SetBinaryValue(v.Name, v.Binary)
		default:
			err = fmt.Errorf("unknown kind %q", v.Kind)
		}
		k.Close()
		if err != nil {
			return fmt.Errorf(`%s\%s: %w`, v.Key, v.Name, err)
		}
	}
	return nil
}
