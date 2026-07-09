package runtime

import (
	heddlecmp "heddle/pkg/lib/cmp"
	heddleenv "heddle/pkg/lib/env"
	heddleio "heddle/pkg/lib/io"
	heddlejson "heddle/pkg/lib/json"
	heddlemath "heddle/pkg/lib/math"
	heddlehttp "heddle/pkg/lib/net/http"
	heddlestrconv "heddle/pkg/lib/strconv"
	heddlestrings "heddle/pkg/lib/strings"
	heddletime "heddle/pkg/lib/time"
)

// PackagesRegistry registra todos os namespaces acessíveis pelo Heddle
var PackagesRegistry = map[string]any{
	"math": map[string]any{
		"round": heddlemath.Round,
		"pow":   heddlemath.Pow,
	},
	"strings": map[string]any{
		"to_upper": heddlestrings.ToUpper,
		"to_lower": heddlestrings.ToLower,
	},
	"strconv": map[string]any{
		"parse_int": heddlestrconv.ParseInt,
	},
	"json": map[string]any{
		"marshal":   heddlejson.Marshal,
		"unmarshal": heddlejson.Unmarshal,
	},
	"io": map[string]any{
		"print": heddleio.Print,
	},
	"cmp": map[string]any{
		"compare": heddlecmp.Compare,
	},
	"time": map[string]any{
		"now":  heddletime.Now,
		"tick": heddletime.Tick,
	},
	"net/http": map[string]any{
		"server": heddlehttp.NewServer,
	},
	"env": map[string]any{
		"string": heddleenv.String,
		"int":    heddleenv.Int,
	},
}

// StdlibMetadataRegistry mapeia os nomes de parâmetros para o ReflectionBridge
var StdlibMetadataRegistry = map[string]FuncMetadata{
	"math.round":        {ParamNames: []string{"x"}},
	"math.pow":          {ParamNames: []string{"x", "y"}},
	"strings.to_upper":  {ParamNames: []string{"s"}},
	"strings.to_lower":  {ParamNames: []string{"s"}},
	"strconv.parse_int": {ParamNames: []string{"s", "base", "bitSize"}},
	"json.marshal":      {ParamNames: []string{"v"}},
	"json.unmarshal":    {ParamNames: []string{"s"}},
	"io.print":          {ParamNames: []string{"val"}},
	"cmp.compare":       {ParamNames: []string{"x", "y"}},
	"time.now":          {ParamNames: []string{}},
	"time.tick":         {ParamNames: []string{"durStr"}},
	"env.string":        {ParamNames: []string{"key"}},
	"env.int":           {ParamNames: []string{"key"}},
}
