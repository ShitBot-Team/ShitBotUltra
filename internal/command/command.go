package command

import (
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/ShitBot-Team/ShitBotUltra/internal/config"
	"github.com/ShitBot-Team/ShitBotUltra/internal/fault"
	"github.com/ShitBot-Team/ShitBotUltra/sdk/adapter"
)

// Split handles quoted Bedrock names without shell expansion, escapes, globbing
// or command substitution. A quote must start and end a complete argument.
func Split(text string) ([]string, error) {
	if len(text) > 4096 {
		return nil, errors.New("command too long")
	}
	args := []string{}
	var token strings.Builder
	var quote rune
	started := false
	closed := false
	flush := func() {
		if started {
			args = append(args, token.String())
			token.Reset()
			started = false
			closed = false
		}
	}
	for _, r := range text {
		if unicode.IsControl(r) {
			return nil, errors.New("control character in command")
		}
		if quote != 0 {
			if r == quote {
				quote = 0
				closed = true
			} else {
				token.WriteRune(r)
			}
			continue
		}
		if unicode.IsSpace(r) {
			flush()
			continue
		}
		if closed {
			return nil, errors.New("characters after closing quote")
		}
		if r == '"' || r == '\'' {
			if started {
				return nil, errors.New("quote inside argument")
			}
			quote = r
			started = true
			continue
		}
		started = true
		token.WriteRune(r)
	}
	if quote != 0 {
		return nil, errors.New("unterminated quote")
	}
	flush()
	if len(args) > 20 {
		return nil, errors.New("too many arguments")
	}
	return args, nil
}

type Service struct {
	mu        sync.Mutex
	cooldowns map[string]time.Time
}

func New() *Service { return &Service{cooldowns: map[string]time.Time{}} }

func (s *Service) Prepare(c config.Command, actor adapter.Actor, player string, args []string) (string, error) {
	if len(args) != len(c.Arguments) {
		return "", fault.New("INVALID_INPUT", "快捷命令参数数量不正确")
	}
	if len(c.Roles) > 0 {
		allowed := false
		for _, role := range c.Roles {
			if slices.Contains(actor.Roles, role) {
				allowed = true
			}
		}
		if !allowed {
			return "", fault.New("FORBIDDEN", "会话身份没有执行此命令的权限")
		}
	}
	if c.RequireBinding && player == "" {
		return "", fault.New("BINDING_REQUIRED", "此命令需要绑定一个玩家")
	}
	// A player placeholder is a single game-command argument. Quoted/Bedrock
	// names remain valid bindings but must not become command syntax.
	if strings.Contains(c.Template, "{player}") && !regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`).MatchString(player) {
		return "", fault.New("INVALID_INPUT", "该玩家名不能安全用于此命令模板")
	}
	replacements := []string{"{player}", player}
	for i, arg := range args {
		if len(arg) > 256 || strings.ContainsAny(arg, "\r\n\x00;{}") || !regexp.MustCompile("^(?:"+c.Arguments[i]+")$").MatchString(arg) {
			return "", fault.New("INVALID_INPUT", "快捷命令参数不符合配置规则")
		}
		replacements = append(replacements, "{"+strconv.Itoa(i)+"}", arg)
	}
	key := c.Name + "\x1f" + actor.Key()
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, until := range s.cooldowns {
		if !now.Before(until) {
			delete(s.cooldowns, key)
		}
	}
	if now.Before(s.cooldowns[key]) {
		return "", fault.New("RATE_LIMITED", "快捷命令仍在冷却中")
	}
	if len(s.cooldowns) >= 10000 {
		return "", fault.New("BUSY", "命令请求过多")
	}
	s.cooldowns[key] = now.Add(time.Duration(c.CooldownSeconds) * time.Second)
	return strings.NewReplacer(replacements...).Replace(c.Template), nil
}
