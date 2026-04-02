#!/usr/bin/env node

const { spawn } = require('child_process');
const path = require('path');
const os = require('os');
const fs = require('fs');

function getBinaryPath() {
  const platform = os.platform();
  let binaryName = 'dp';
  if (platform === 'win32') {
    binaryName = 'dp.exe';
  }

  const binaryPath = path.join(__dirname, binaryName);

  if (!fs.existsSync(binaryPath)) {
    console.error(`Error: dp binary not found at ${binaryPath}`);
    console.error('Run: npm rebuild devpit');
    process.exit(1);
  }

  return binaryPath;
}

function main() {
  const binaryPath = getBinaryPath();

  const child = spawn(binaryPath, process.argv.slice(2), {
    stdio: 'inherit',
    env: process.env
  });

  child.on('error', (err) => {
    console.error(`Error executing dp: ${err.message}`);
    process.exit(1);
  });

  child.on('exit', (code, signal) => {
    if (signal) {
      process.exit(1);
    }
    process.exit(code || 0);
  });
}

main();
