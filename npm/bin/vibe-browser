#!/usr/bin/env node

// Wrapper script that resolves and executes the platform-specific binary.
// When installed via `npm i -g @startvibecoding/vibe-browser`, this script finds the
// correct binary from the platform-specific optional dependency package.

const { execFileSync } = require('child_process');
const path = require('path');
const fs = require('fs');

const SCOPE = '@startvibecoding';
const PKG_BASE = 'vibe-browser';

// Map npm os/cpu to package name
const PLATFORM_MAP = {
  'linux-x64-glibc': `${SCOPE}/${PKG_BASE}-linux-x64`,
  'linux-arm64-glibc': `${SCOPE}/${PKG_BASE}-linux-arm64`,
  'linux-x64-musl': `${SCOPE}/${PKG_BASE}-linux-musl-x64`,
  'linux-arm64-musl': `${SCOPE}/${PKG_BASE}-linux-musl-arm64`,
  'darwin-x64': `${SCOPE}/${PKG_BASE}-darwin-x64`,
  'darwin-arm64': `${SCOPE}/${PKG_BASE}-darwin-arm64`,
  'win32-x64': `${SCOPE}/${PKG_BASE}-win32-x64`,
  'win32-arm64': `${SCOPE}/${PKG_BASE}-win32-arm64`,
};

function detectPlatform() {
  const os = process.platform;
  const arch = process.arch;

  if (os === 'linux') {
    const isMusl = (() => {
      try {
        if (fs.existsSync('/etc/alpine-release')) return true;
        const { execSync } = require('child_process');
        const output = execSync('ldd --version 2>&1 || true', { encoding: 'utf8' });
        return output.includes('musl');
      } catch {
        return false;
      }
    })();
    return `${os}-${arch}-${isMusl ? 'musl' : 'glibc'}`;
  }

  return `${os}-${arch}`;
}

function findBinary() {
  const platform = detectPlatform();
  const packageName = PLATFORM_MAP[platform];

  if (!packageName) {
    console.error(`Unsupported platform: ${platform}`);
    console.error(`Supported platforms: ${Object.keys(PLATFORM_MAP).join(', ')}`);
    process.exit(1);
  }

  const searchDirs = [];
  const addSearchDir = (dir) => {
    if (dir && !searchDirs.includes(dir)) {
      searchDirs.push(dir);
    }
  };

  try {
    addSearchDir(path.dirname(require.resolve(`${packageName}/package.json`)));
  } catch {}

  // Scoped packages may be in node_modules/@scope/pkg-name
  const pkgShortName = packageName.replace(`${SCOPE}/`, '');
  addSearchDir(path.join(__dirname, '..', 'node_modules', SCOPE, pkgShortName));
  addSearchDir(path.join(__dirname, '..', '..', pkgShortName));
  addSearchDir(path.join(__dirname, '..', '..', SCOPE, pkgShortName));

  for (const pkgDir of searchDirs) {
    const binName = process.platform === 'win32' ? 'vibe-browser.exe' : 'vibe-browser';
    const binPath = path.join(pkgDir, 'bin', binName);

    if (fs.existsSync(binPath)) {
      return binPath;
    }
  }

  // Fallback: check if there's a binary directly in the main package's bin/
  const fallbackBinName = (() => {
    const suffix = process.platform === 'win32' ? '.exe' : '';
    const osMap = { linux: 'linux', darwin: 'darwin', win32: 'windows' };
    const archMap = { x64: 'amd64', arm64: 'arm64' };
    return `vibe-browser-${osMap[process.platform]}-${archMap[process.arch]}${suffix}`;
  })();

  const fallbackPath = path.join(__dirname, fallbackBinName);
  if (fs.existsSync(fallbackPath)) {
    return fallbackPath;
  }

  console.error(`Could not find vibe-browser binary for platform: ${detectPlatform()}`);
  console.error(`Searched for package: ${packageName}`);
  console.error(`Searched in: ${searchDirs.join(', ')}`);
  console.error('');
  console.error('If you installed globally, try reinstalling:');
  console.error('  npm install -g @startvibecoding/vibe-browser');
  console.error('');
  console.error('If the problem persists, install via Go:');
  console.error('  go install github.com/startvibecoding/vibe-browser/cmd/vibe-browser@latest');
  process.exit(1);
}

const binaryPath = findBinary();
const args = process.argv.slice(2);

try {
  execFileSync(binaryPath, args, { stdio: 'inherit' });
} catch (err) {
  if (err.status !== undefined) {
    process.exit(err.status);
  }
  process.exit(1);
}
