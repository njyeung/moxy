package config

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

const templatePart1 = `table ip moxy
delete table ip moxy

table ip moxy {
	chain prerouting {
		type nat hook prerouting priority dstnat; policy accept;
`
const templatePart2 = `	}

	chain forward {
		type filter hook forward priority filter; policy accept;
		oifname %[1]q tcp flags syn tcp option maxseg size set 1240
		iifname %[1]q tcp flags syn tcp option maxseg size set 1240
	}
}
`

var hashRe = regexp.MustCompile(`comment "sha256:([0-9a-f]+)"`)

func NftString(cfg Config, wanIface, wgIface string) (string, error) {
	hash, err := Hash(cfg)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString(templatePart1)
	fmt.Fprintf(&sb, "\t\tcomment %q;\n\n", "sha256:"+hash)

	for _, vm := range cfg.ExposedVms {
		for _, bind := range vm.Binds {
			fmt.Fprintf(&sb, "\t\tiifname %q %s dport %d dnat to %s comment %q\n",
				wanIface, bind.Protocol, bind.VpsPort, bind.VmAddrPort, fmt.Sprintf("vm %d %s", vm.VmId, vm.Name))
		}
	}

	fmt.Fprintf(&sb, templatePart2, wgIface)

	return sb.String(), nil
}

// ReadHash returns "" if there's no hash comment
func ReadHash(nftString string) string {
	m := hashRe.FindStringSubmatch(nftString)
	if m == nil {
		return ""
	}
	return m[1]
}

func Hash(cfg Config) (string, error) {
	sorted := make([]ExposedVM, len(cfg.ExposedVms))
	for i, vm := range cfg.ExposedVms {
		vm.Binds = slices.Clone(vm.Binds)
		slices.SortFunc(vm.Binds, func(a, b *Bind) int {
			return cmp.Or(
				cmp.Compare(a.Protocol, b.Protocol),
				cmp.Compare(a.VpsPort, b.VpsPort),
			)
		})
		sorted[i] = vm
	}
	slices.SortFunc(sorted, func(a, b ExposedVM) int {
		return cmp.Compare(a.VmId, b.VmId)
	})

	b, err := json.Marshal(sorted)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
