import Flutter
import UIKit

#if canImport(ParleyCore)
import ParleyCore
#endif

@main
@objc class AppDelegate: FlutterAppDelegate, FlutterImplicitEngineDelegate {
  override func application(
    _ application: UIApplication,
    didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]?
  ) -> Bool {
    return super.application(application, didFinishLaunchingWithOptions: launchOptions)
  }

  func didInitializeImplicitFlutterEngine(_ engineBridge: FlutterImplicitEngineBridge) {
    GeneratedPluginRegistrant.register(with: engineBridge.pluginRegistry)

    guard let registrar = engineBridge.pluginRegistry.registrar(forPlugin: "ParleyCoreHost") else {
      return
    }
    // Device Host channel: the Go core runs in this process and serves the wire
    // protocol on loopback. The config JSON holds Provider keys; it is passed
    // straight to Go and never logged.
    let channel = FlutterMethodChannel(
      name: "app.parley/core",
      binaryMessenger: registrar.messenger()
    )
    channel.setMethodCallHandler { call, result in
      switch call.method {
      case "start":
        #if canImport(ParleyCore)
        do {
          result(try MobileStart(call.arguments as? String ?? ""))
        } catch {
          // Go's Start errors are already redacted; the Dart side redacts the
          // Provider values again before showing this to the user.
          result(FlutterError(
            code: "start",
            message: (error as NSError).localizedDescription,
            details: nil
          ))
        }
        #else
        result(FlutterError(
          code: "unavailable",
          message: "The on-device host is not linked in this build.",
          details: nil
        ))
        #endif
      case "stop":
        #if canImport(ParleyCore)
        MobileStop()
        #endif
        result(nil)
      default:
        result(FlutterMethodNotImplemented)
      }
    }
  }
}
