package cisco_ios_ssh

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"golang.org/x/crypto/ssh"
)

var (
	promptPattern     = regexp.MustCompile(`(?m)[^\r\n]+[>#]\s*$`)
	passwordPattern   = regexp.MustCompile(`(?i)password:\s*$`)
	ansiPattern       = regexp.MustCompile(`\x1b(?:[@-_][0-?]*[ -/]*[@-~]|\[[0-?]*[ -/]*[@-~])`)
	ciscoErrorPattern = regexp.MustCompile(`(?mi)^%\s*(invalid input|incomplete command|ambiguous command|authorization failed|access denied|error)`)
)

func executeShell(ctx context.Context, client *ssh.Client, enableSecret, command string, maxOutput int64) (string, error) {
	streamCtx, stopStream := context.WithCancel(ctx)
	defer stopStream()
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	stdin, err := session.StdinPipe()
	if err != nil {
		return "", err
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return "", err
	}
	if err := session.RequestPty("vt100", 80, 240, ssh.TerminalModes{ssh.ECHO: 0}); err != nil {
		return "", err
	}
	if err := session.Shell(); err != nil {
		return "", err
	}
	stream := pump(streamCtx, stdout, stderr)
	initial, err := readUntil(ctx, stream, maxOutput, promptPattern.Match)
	if err != nil {
		return "", err
	}
	prompt := lastPrompt(initial)
	if strings.HasSuffix(strings.TrimSpace(prompt), ">") && enableSecret != "" {
		if _, err := io.WriteString(stdin, "enable\n"); err != nil {
			return "", err
		}
		challenge, err := readUntil(ctx, stream, 64*1024, func(value []byte) bool { return passwordPattern.Match(value) || promptPattern.Match(value) })
		if err != nil {
			return "", err
		}
		if passwordPattern.Match(challenge) {
			if _, err := io.WriteString(stdin, enableSecret+"\n"); err != nil {
				return "", err
			}
			enabled, err := readUntil(ctx, stream, 64*1024, promptPattern.Match)
			if err != nil {
				return "", err
			}
			prompt = lastPrompt(enabled)
			if !strings.HasSuffix(strings.TrimSpace(prompt), "#") {
				return "", fmt.Errorf("%w: enable authentication rejected", domain.ErrDeviceAuth)
			}
		}
	}
	if _, err := io.WriteString(stdin, "terminal length 0\n"); err != nil {
		return "", err
	}
	if _, err := readUntil(ctx, stream, 64*1024, exactPrompt(prompt)); err != nil {
		return "", err
	}
	if _, err := io.WriteString(stdin, command+"\n"); err != nil {
		return "", err
	}
	raw, err := readUntil(ctx, stream, maxOutput, exactPrompt(prompt))
	if err != nil {
		return "", err
	}
	output := sanitizeOutput(string(raw), command)
	if ciscoErrorPattern.MatchString(output) {
		return "", errors.New("cisco CLI rejected fixed operation")
	}
	return output, nil
}

func pump(ctx context.Context, readers ...io.Reader) <-chan byte {
	result := make(chan byte, 4096)
	var remaining = make(chan struct{}, len(readers))
	for _, reader := range readers {
		go func(r io.Reader) {
			scanner := bufio.NewReader(r)
			for {
				value, err := scanner.ReadByte()
				if err != nil {
					break
				}
				select {
				case result <- value:
				case <-ctx.Done():
					remaining <- struct{}{}
					return
				}
			}
			remaining <- struct{}{}
		}(reader)
	}
	go func() {
		for range readers {
			<-remaining
		}
		close(result)
	}()
	return result
}

func exactPrompt(prompt string) func([]byte) bool {
	prompt = strings.TrimSpace(prompt)
	return func(value []byte) bool { return strings.HasSuffix(strings.TrimSpace(string(value)), prompt) }
}

func readUntil(ctx context.Context, input <-chan byte, limit int64, complete func([]byte) bool) ([]byte, error) {
	var result bytes.Buffer
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("%w: %v", domain.ErrTimeout, ctx.Err())
		case value, ok := <-input:
			if !ok {
				return nil, io.ErrUnexpectedEOF
			}
			if int64(result.Len()+1) > limit {
				return nil, domain.ErrOutputTooLarge
			}
			result.WriteByte(value)
			if (value == '>' || value == '#' || value == ':' || value == ' ') && complete(result.Bytes()) {
				return result.Bytes(), nil
			}
		}
	}
}

func lastPrompt(value []byte) string {
	matches := promptPattern.FindAll(value, -1)
	if len(matches) == 0 {
		return ""
	}
	return string(matches[len(matches)-1])
}

func sanitizeOutput(raw, command string) string {
	raw = ansiPattern.ReplaceAllString(raw, "")
	raw = strings.ReplaceAll(raw, "\r", "")
	lines := strings.Split(raw, "\n")
	cleaned := make([]string, 0, len(lines))
	removedCommand := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !removedCommand && (trimmed == command || strings.HasSuffix(trimmed, command)) {
			removedCommand = true
			continue
		}
		cleaned = append(cleaned, line)
	}
	for len(cleaned) > 0 && (strings.TrimSpace(cleaned[len(cleaned)-1]) == "" || promptPattern.MatchString(cleaned[len(cleaned)-1])) {
		cleaned = cleaned[:len(cleaned)-1]
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n")) + "\n"
}
