#!/usr/bin/env node
import { cp, mkdir, readFile, rm, stat, writeFile } from 'node:fs/promises'
import path from 'node:path'
import process from 'node:process'
import { fileURLToPath, pathToFileURL } from 'node:url'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')

export function updateAssetNames(version) {
  const prefix = `evergate_${version}`
  return {
    windows: `${prefix}_windows_amd64_setup.exe`,
    macOS: `${prefix}_macos_arm64_setup.dmg`,
    linux: `${prefix}_linux_amd64.tar.gz`,
    checksums: `${prefix}_checksums.txt`,
  }
}

export async function stageUpdatePackage(version, releaseDir, outputDir) {
  version = String(version).replace(/^v/, '')
  if (!/^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/.test(version)) {
    throw new Error(`invalid semver version: ${version}`)
  }
  releaseDir = path.resolve(releaseDir)
  outputDir = path.resolve(outputDir)
  const names = updateAssetNames(version)
  const required = [names.windows, names.linux, names.checksums]
  for (const name of required) await stat(path.join(releaseDir, name))

  const artifacts = [...required]
  try {
    await stat(path.join(releaseDir, names.macOS))
    artifacts.push(names.macOS)
  } catch (error) {
    if (error?.code !== 'ENOENT') throw error
  }

  await rm(outputDir, { recursive: true, force: true })
  await mkdir(path.join(outputDir, 'artifacts'), { recursive: true })
  for (const name of artifacts) {
    await cp(path.join(releaseDir, name), path.join(outputDir, 'artifacts', name))
  }
  await cp(path.join(root, 'LICENSE'), path.join(outputDir, 'LICENSE'))
  await cp(path.join(root, 'npm/updates/README.md'), path.join(outputDir, 'README.md'))
  const template = JSON.parse(await readFile(path.join(root, 'npm/updates/package.json'), 'utf8'))
  await writeFile(
    path.join(outputDir, 'package.json'),
    `${JSON.stringify({ ...template, version }, null, 2)}\n`,
  )
  return { artifacts, outputDir }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const [version, releaseDir, outputDir] = process.argv.slice(2)
  if (!version || !releaseDir || !outputDir) {
    console.error('usage: node scripts/stage-update-npm.mjs <version> <release-dir> <output-dir>')
    process.exit(2)
  }
  try {
    const result = await stageUpdatePackage(version, releaseDir, outputDir)
    console.log(`staged @heguoliang/evergate-updates@${version.replace(/^v/, '')}: ${result.artifacts.join(', ')}`)
  } catch (error) {
    console.error(error.message)
    process.exit(1)
  }
}
