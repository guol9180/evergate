import AppKit
import Foundation

private let appBundleID = "com.evergate.app"
private let installDirectory = FileManager.default.homeDirectoryForCurrentUser
  .appendingPathComponent("Applications", isDirectory: true)
private let installedApp = installDirectory.appendingPathComponent("EverGate.app", isDirectory: true)
private let launchAgent = FileManager.default.homeDirectoryForCurrentUser
  .appendingPathComponent("Library/LaunchAgents/com.evergate.app.plist")

final class InstallerDelegate: NSObject, NSApplicationDelegate {
  private var window: NSWindow!
  private var statusLabel: NSTextField!
  private var licenseText: NSTextView!
  private var autostart: NSButton!
  private var launchAfterInstall: NSButton!
  private var installButton: NSButton!
  private var closeButton: NSButton!
  private var progress: NSProgressIndicator!

  func applicationDidFinishLaunching(_ notification: Notification) {
    NSApp.setActivationPolicy(.accessory)
    buildWindow()
    window.center()
    window.makeKeyAndOrderFront(nil)
    NSApp.activate(ignoringOtherApps: true)
  }

  func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { true }

  private func buildWindow() {
    window = NSWindow(
      contentRect: NSRect(x: 0, y: 0, width: 620, height: 560),
      styleMask: [.titled, .closable],
      backing: .buffered,
      defer: false
    )
    window.title = "EverGate 安装器"
    window.minSize = NSSize(width: 620, height: 560)

    let root = NSView(frame: window.contentView!.bounds)
    root.autoresizingMask = [.width, .height]
    window.contentView = root

    let title = NSTextField(labelWithString: "安装 EverGate")
    title.font = .systemFont(ofSize: 26, weight: .semibold)
    title.frame = NSRect(x: 32, y: 500, width: 556, height: 34)
    root.addSubview(title)

    statusLabel = NSTextField(labelWithString: statusText())
    statusLabel.font = .systemFont(ofSize: 13)
    statusLabel.textColor = .secondaryLabelColor
    statusLabel.lineBreakMode = .byWordWrapping
    statusLabel.maximumNumberOfLines = 3
    statusLabel.frame = NSRect(x: 32, y: 448, width: 556, height: 42)
    root.addSubview(statusLabel)

    let licenseTitle = NSTextField(labelWithString: "许可协议")
    licenseTitle.font = .systemFont(ofSize: 13, weight: .semibold)
    licenseTitle.frame = NSRect(x: 32, y: 412, width: 556, height: 20)
    root.addSubview(licenseTitle)

    let scroll = NSScrollView(frame: NSRect(x: 32, y: 228, width: 556, height: 176))
    scroll.hasVerticalScroller = true
    scroll.borderType = .bezelBorder
    licenseText = NSTextView(frame: scroll.bounds)
    licenseText.isEditable = false
    licenseText.isSelectable = true
    licenseText.drawsBackground = true
    licenseText.font = .monospacedSystemFont(ofSize: 11, weight: .regular)
    licenseText.string = licenseBody()
    scroll.documentView = licenseText
    root.addSubview(scroll)

    autostart = NSButton(checkboxWithTitle: "登录 macOS 时自动启动 EverGate", target: nil, action: nil)
    autostart.frame = NSRect(x: 32, y: 188, width: 556, height: 22)
    root.addSubview(autostart)

    launchAfterInstall = NSButton(checkboxWithTitle: "安装完成后立即启动 EverGate", target: nil, action: nil)
    launchAfterInstall.state = .on
    launchAfterInstall.frame = NSRect(x: 32, y: 160, width: 556, height: 22)
    root.addSubview(launchAfterInstall)

    progress = NSProgressIndicator(frame: NSRect(x: 32, y: 130, width: 556, height: 12))
    progress.style = .bar
    progress.isIndeterminate = true
    progress.isHidden = true
    root.addSubview(progress)

    installButton = NSButton(title: "安装", target: self, action: #selector(install))
    installButton.bezelStyle = .rounded
    installButton.keyEquivalent = "\r"
    installButton.frame = NSRect(x: 486, y: 32, width: 102, height: 32)
    root.addSubview(installButton)

    closeButton = NSButton(title: "取消", target: self, action: #selector(cancel))
    closeButton.bezelStyle = .rounded
    closeButton.frame = NSRect(x: 396, y: 32, width: 82, height: 32)
    root.addSubview(closeButton)
  }

  private func statusText() -> String {
    if FileManager.default.fileExists(atPath: installedApp.path) {
      return "检测到已安装的 EverGate，将升级 \(installedApp.path)。配置与数据库位于 ~/Library/Application Support/EverGate，升级不会覆盖或删除已有数据。"
    }
    return "将首次安装到 \(installedApp.path)。配置与数据库会创建在 ~/Library/Application Support/EverGate。"
  }

  private func licenseBody() -> String {
    let resourceRoot = Bundle.main.resourceURL!
    let candidates = [
      resourceRoot.appendingPathComponent("LICENSE"),
      resourceRoot.appendingPathComponent("EverGate.app/Contents/Resources/LICENSE"),
    ]
    for path in candidates {
      if let body = try? String(contentsOf: path, encoding: .utf8) { return body }
    }
    return "MIT License\n\nCopyright (c) 2026 guol9180"
  }

  @objc private func cancel() {
    NSApp.terminate(nil)
  }

  @objc private func install() {
    installButton.isEnabled = false
    closeButton.isEnabled = false
    autostart.isEnabled = false
    launchAfterInstall.isEnabled = false
    progress.isHidden = false
    progress.startAnimation(nil)
    statusLabel.stringValue = "正在安装，请稍候…"

    DispatchQueue.global(qos: .userInitiated).async {
      do {
        try self.performInstall(enableAutostart: self.autostart.state == .on)
        DispatchQueue.main.async {
          self.progress.stopAnimation(nil)
          self.progress.isHidden = true
          self.statusLabel.stringValue = "安装完成。EverGate 已安装到 \(installedApp.path)。"
          self.installButton.title = "完成"
          self.installButton.action = #selector(self.finish)
          self.installButton.isEnabled = true
          self.closeButton.isHidden = true
          if self.launchAfterInstall.state == .on {
            NSWorkspace.shared.open(installedApp)
          }
        }
      } catch {
        DispatchQueue.main.async {
          self.progress.stopAnimation(nil)
          self.progress.isHidden = true
          self.statusLabel.stringValue = "安装失败：\(error.localizedDescription)"
          self.installButton.isEnabled = true
          self.closeButton.isEnabled = true
          self.autostart.isEnabled = true
          self.launchAfterInstall.isEnabled = true
          let alert = NSAlert()
          alert.alertStyle = .critical
          alert.messageText = "EverGate 安装失败"
          alert.informativeText = error.localizedDescription
          alert.runModal()
        }
      }
    }
  }

  @objc private func finish() {
    NSApp.terminate(nil)
  }

  private func performInstall(enableAutostart: Bool) throws {
    let sourceApp = Bundle.main.resourceURL!.appendingPathComponent("EverGate.app", isDirectory: true)
    guard FileManager.default.fileExists(atPath: sourceApp.path) else {
      throw NSError(domain: "EverGateInstaller", code: 1,
                    userInfo: [NSLocalizedDescriptionKey: "安装器内缺少 EverGate.app"])
    }

    NSRunningApplication.runningApplications(withBundleIdentifier: appBundleID)
      .forEach { $0.terminate() }
    try FileManager.default.createDirectory(at: installDirectory, withIntermediateDirectories: true)
    let staging = installDirectory.appendingPathComponent(".EverGate.app.new-\(UUID().uuidString)")
    try? FileManager.default.removeItem(at: staging)
    try FileManager.default.copyItem(at: sourceApp, to: staging)
    if FileManager.default.fileExists(atPath: installedApp.path) {
      _ = try FileManager.default.replaceItemAt(installedApp, withItemAt: staging)
    } else {
      try FileManager.default.moveItem(at: staging, to: installedApp)
    }

    if enableAutostart {
      try writeLaunchAgent()
    } else {
      try? FileManager.default.removeItem(at: launchAgent)
    }
  }

  private func writeLaunchAgent() throws {
    try FileManager.default.createDirectory(
      at: launchAgent.deletingLastPathComponent(),
      withIntermediateDirectories: true
    )
    let executable = installedApp.appendingPathComponent("Contents/MacOS/evergate").path
    let plist: [String: Any] = [
      "Label": appBundleID,
      "ProgramArguments": [executable, "--autostart"],
      "RunAtLoad": true,
      "KeepAlive": false,
      "ProcessType": "Background",
    ]
    let data = try PropertyListSerialization.data(fromPropertyList: plist, format: .xml, options: 0)
    try data.write(to: launchAgent, options: .atomic)
  }
}

let app = NSApplication.shared
let delegate = InstallerDelegate()
app.delegate = delegate
app.run()
