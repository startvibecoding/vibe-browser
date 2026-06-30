#!/usr/bin/env node

// Skip postinstall output in CI or when suppressed
if (process.env.CI || process.env.npm_config_yes || process.env.VIBE_BROWSER_SKIP_POSTINSTALL) {
  process.exit(0);
}

const os = require('os');
const path = require('path');

const RESET  = '\x1b[0m';
const BOLD   = '\x1b[1m';
const DIM    = '\x1b[2m';
const CYAN   = '\x1b[36m';
const BRIGHT_CYAN = '\x1b[96m';
const WHITE  = '\x1b[97m';

const logo = [
  '                __     ',
  ' _   _____ ___/ /___ _',
  ' | | / / _ `/ __/ __ `/',
  ' | |/ / /_/ / / / /_/ /',
  ' |___/\\__,_/_/  \\__,_/ ',
].join('\n');

function pkgVersion() {
  try {
    return require('../package.json').version;
  } catch {
    return '';
  }
}

const ver = pkgVersion();
const verStr = ver ? ` ${DIM}v${ver}${RESET}` : '';

console.log();
console.log(`${BRIGHT_CYAN}${BOLD}${logo}${RESET}${verStr}`);
console.log();
console.log(`  ${BOLD}${WHITE}Fast browser automation CLI for AI agents${RESET}`);
console.log();
console.log(`  ${DIM}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}`);
console.log();
console.log(`  ${BOLD}Quick Start${RESET}`);
console.log();
console.log(`    vibe-browser open https://example.com     ${DIM}Open a page${RESET}`);
console.log(`    vibe-browser snapshot -i                   ${DIM}See interactive elements${RESET}`);
console.log(`    vibe-browser click "button"                ${DIM}Click an element${RESET}`);
console.log(`    vibe-browser screenshot -o page.png        ${DIM}Take screenshot${RESET}`);
console.log();
console.log(`  ${BOLD}Docs${RESET}   ${CYAN}https://github.com/startvibecoding/vibe-browser${RESET}`);
console.log();
