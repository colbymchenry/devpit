#!/usr/bin/env node
const { execSync } = require('child_process');
const fs = require('fs');
const path = require('path');
const readline = require('readline');

const PKG_PATH = path.join(__dirname, 'package.json');
const pkg = JSON.parse(fs.readFileSync(PKG_PATH, 'utf-8'));
const [major, minor, patch] = pkg.version.split('.').map(Number);

const rl = readline.createInterface({ input: process.stdin, output: process.stdout });

function ask(question) {
  return new Promise((resolve) => rl.question(question, resolve));
}

async function main() {
  console.log(`\nCurrent version: ${pkg.version}\n`);
  console.log('  1) patch  -> ' + `${major}.${minor}.${patch + 1}`);
  console.log('  2) minor  -> ' + `${major}.${minor + 1}.0`);
  console.log('  3) major  -> ' + `${major + 1}.0.0`);
  console.log('');

  const choice = await ask('Bump version (1/2/3): ');

  let bump;
  switch (choice.trim()) {
    case '1': bump = 'patch'; break;
    case '2': bump = 'minor'; break;
    case '3': bump = 'major'; break;
    default:
      console.log('Invalid choice. Exiting.');
      rl.close();
      process.exit(1);
  }

  // Bump version in package.json
  execSync(`npm version ${bump} --no-git-tag-version`, { stdio: 'inherit', cwd: __dirname });

  const updated = JSON.parse(fs.readFileSync(PKG_PATH, 'utf-8'));
  console.log(`\nVersion bumped to ${updated.version}`);

  const confirm = await ask(`Publish ${updated.name}@${updated.version} to npm? (y/n): `);
  if (confirm.trim().toLowerCase() !== 'y') {
    console.log('Aborted.');
    rl.close();
    process.exit(0);
  }

  console.log('\nPublishing...');
  execSync('npm publish --access public', { stdio: 'inherit', cwd: __dirname });

  console.log(`\nPublished ${updated.name}@${updated.version}`);
  rl.close();
}

main().catch((err) => {
  console.error(err);
  rl.close();
  process.exit(1);
});
