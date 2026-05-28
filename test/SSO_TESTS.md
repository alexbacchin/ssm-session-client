# SSO Acceptance Tests

## Quick Commands

### Test SSO with ssh-direct (interactive, requires browser approval)
```bash
export SSC_SSO_TEST_PROFILE=sandbox-sso
export SSC_SSO_TEST_INTERACTIVE=true
go test ./test/acceptance -tags acceptance -run TestSSOLoginInteractive -race -v -timeout 15m
```

## Test Overview

| Test | Type | Requirements | CI |
|------|------|--------------|-----|
| `TestSSOLoginWithCachedToken` | Integration | Infrastructure + cached credentials | ✅ Yes |
| `TestSSOLoginInteractive` | Integration + Interactive | Infrastructure + `SSC_SSO_TEST_INTERACTIVE=true` | ❌ Manual only |

## Manual Testing

### Test sso-login command directly
```bash
export AWS_PROFILE=sandbox-sso
./ssm-session-client --aws-profile sandbox-sso sso-login --timeout 300
# Opens browser auth URL — approve in browser to complete login
```

### Test sso-logout command directly
```bash
export AWS_PROFILE=sandbox-sso
./ssm-session-client --aws-profile sandbox-sso sso-logout
```

## Notes

- `sso-login` and `sso-logout` commands are tested manually only (not in CI/acceptance tests)
- The acceptance tests (`TestSSOLoginWithCachedToken`, `TestSSOLoginInteractive`) test the `--sso-login` **flag** with SSH, not the standalone `sso-login` command
