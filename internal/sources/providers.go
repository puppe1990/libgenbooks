package sources

import (
	"context"
	"fmt"
	"strings"

	"github.com/puppe1990/libgenbooks/internal/annas"
	"github.com/puppe1990/libgenbooks/internal/ipfs"
	"github.com/puppe1990/libgenbooks/internal/libgen"
)

// Provider names accepted in LIBGEN_SOURCES.
const (
	NameLibgen = "libgen"
	NameAnnas  = "annas"
)

// DefaultNames is the provider chain a fresh install runs, in preference order.
var DefaultNames = []string{NameLibgen, NameAnnas}

// validNames backs the error message for a typo in LIBGEN_SOURCES.
var validNames = []string{NameLibgen, NameAnnas}

// Providers builds the named providers from live clients. Unknown names are
// an error so a typo in LIBGEN_SOURCES fails loudly instead of silently
// disabling a source.
func Providers(names []string, libgenClient *libgen.Client, annasClient *annas.Client, ipfsClient *ipfs.Client) ([]Provider, error) {
	if len(names) == 0 {
		names = DefaultNames
	}
	built := make([]Provider, 0, len(names))
	for _, name := range names {
		normalized := strings.ToLower(strings.TrimSpace(name))
		switch normalized {
		case NameLibgen:
			if libgenClient == nil {
				return nil, fmt.Errorf("sources: provider %q needs a libgen client", normalized)
			}
			built = append(built, libgenProvider{client: libgenClient})
		case NameAnnas:
			if annasClient == nil || ipfsClient == nil {
				return nil, fmt.Errorf("sources: provider %q needs the annas and ipfs clients", normalized)
			}
			built = append(built, annasProvider{annas: annasClient, ipfs: ipfsClient})
		default:
			return nil, fmt.Errorf("sources: unknown provider %q (valid: %s)", normalized, strings.Join(validNames, ", "))
		}
	}
	if len(built) == 0 {
		return nil, fmt.Errorf("sources: no provider enabled (valid: %s)", strings.Join(validNames, ", "))
	}
	return built, nil
}

// libgenProvider resolves the signed get.php link through the mirror handshake.
type libgenProvider struct {
	client *libgen.Client
}

func (p libgenProvider) Name() string { return NameLibgen }

func (p libgenProvider) URLs(ctx context.Context, md5 string) ([]string, error) {
	link, err := p.client.ResolveDownload(ctx, md5)
	if err != nil {
		return nil, err
	}
	return []string{link}, nil
}

// annasProvider reads the IPFS CID from Anna's Archive and races the public
// gateways, so a missing libgen link still yields downloadable URLs.
type annasProvider struct {
	annas *annas.Client
	ipfs  *ipfs.Client
}

func (p annasProvider) Name() string { return NameAnnas }

func (p annasProvider) URLs(ctx context.Context, md5 string) ([]string, error) {
	cid, err := p.annas.CID(ctx, md5)
	if err != nil {
		return nil, err
	}
	return p.ipfs.Resolve(ctx, cid)
}
