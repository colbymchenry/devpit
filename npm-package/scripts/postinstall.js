#!/usr/bin/env node

const https = require('https');
const fs = require('fs');
const path = require('path');
const os = require('os');
const { execSync } = require('child_process');

const packageJson = require('../package.json');
const VERSION = packageJson.version;
const REPO = 'colbymchenry/devpit';

function getPlatformInfo() {
  const platform = os.platform();
  const arch = os.arch();

  let platformName, archName;
  let binaryName = 'dp';

  switch (platform) {
    case 'darwin':  platformName = 'darwin'; break;
    case 'linux':   platformName = 'linux'; break;
    case 'win32':   platformName = 'windows'; binaryName = 'dp.exe'; break;
    default: throw new Error(`Unsupported platform: ${platform}`);
  }

  switch (arch) {
    case 'x64':   archName = 'amd64'; break;
    case 'arm64': archName = 'arm64'; break;
    default: throw new Error(`Unsupported architecture: ${arch}`);
  }

  return { platformName, archName, binaryName };
}

function downloadFile(url, dest) {
  return new Promise((resolve, reject) => {
    console.log(`Downloading: ${url}`);
    const file = fs.createWriteStream(dest);

    const request = https.get(url, (response) => {
      if (response.statusCode === 301 || response.statusCode === 302) {
        downloadFile(response.headers.location, dest).then(resolve).catch(reject);
        return;
      }
      if (response.statusCode !== 200) {
        reject(new Error(`HTTP ${response.statusCode}`));
        return;
      }
      response.pipe(file);
      file.on('finish', () => {
        file.close((err) => { if (err) reject(err); else resolve(); });
      });
    });

    request.on('error', (err) => { fs.unlink(dest, () => {}); reject(err); });
    file.on('error', (err) => { fs.unlink(dest, () => {}); reject(err); });
  });
}

function extractArchive(archivePath, destDir, binaryName, isZip) {
  if (isZip) {
    if (os.platform() === 'win32') {
      execSync(`powershell -command "Expand-Archive -Path '${archivePath}' -DestinationPath '${destDir}' -Force"`, { stdio: 'inherit' });
    } else {
      execSync(`unzip -o "${archivePath}" -d "${destDir}"`, { stdio: 'inherit' });
    }
  } else {
    execSync(`tar -xzf "${archivePath}" -C "${destDir}"`, { stdio: 'inherit' });
  }

  const extractedBinary = path.join(destDir, binaryName);
  if (!fs.existsSync(extractedBinary)) {
    throw new Error(`Binary not found after extraction: ${extractedBinary}`);
  }

  if (os.platform() !== 'win32') {
    fs.chmodSync(extractedBinary, 0o755);
  }
}

async function install() {
  try {
    const { platformName, archName, binaryName } = getPlatformInfo();
    console.log(`Installing dp v${VERSION} for ${platformName}-${archName}...`);

    const isZip = platformName === 'windows';
    const ext = isZip ? 'zip' : 'tar.gz';
    const archiveName = `devpit_${VERSION}_${platformName}_${archName}.${ext}`;
    const downloadUrl = `https://github.com/${REPO}/releases/download/v${VERSION}/${archiveName}`;

    const binDir = path.join(__dirname, '..', 'bin');
    const archivePath = path.join(binDir, archiveName);

    if (!fs.existsSync(binDir)) {
      fs.mkdirSync(binDir, { recursive: true });
    }

    await downloadFile(downloadUrl, archivePath);
    extractArchive(archivePath, binDir, binaryName, isZip);
    fs.unlinkSync(archivePath);

    try {
      const output = execSync(`"${path.join(binDir, binaryName)}" --version`, { encoding: 'utf8' });
      console.log(`dp installed: ${output.trim()}`);
    } catch (err) {
      console.warn('Warning: Could not verify binary version');
    }

  } catch (err) {
    console.error(`Error installing dp: ${err.message}`);
    console.error('');
    console.error('Manual install:');
    console.error(`  https://github.com/${REPO}/releases`);
    process.exit(1);
  }
}

if (!process.env.CI) {
  install();
} else {
  console.log('Skipping binary download in CI');
}
