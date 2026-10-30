package app.parley.translate

import android.Manifest
import android.content.pm.PackageManager
import android.os.Build
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel
import mobile.Mobile

class MainActivity : FlutterActivity() {
    private var pending: MethodChannel.Result? = null

    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)
        // Device Host channel: the Go core runs in this process and serves the
        // wire protocol on loopback. The config JSON holds Provider keys; it is
        // passed straight to Go and never logged.
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, CORE_CHANNEL)
            .setMethodCallHandler { call, result ->
                when (call.method) {
                    "start" -> try {
                        result.success(Mobile.start(call.arguments as? String ?: ""))
                    } catch (error: Throwable) {
                        result.error(
                            "start",
                            error.message ?: "The on-device host could not start.",
                            null,
                        )
                    }
                    "stop" -> {
                        Mobile.stop()
                        result.success(null)
                    }
                    else -> result.notImplemented()
                }
            }
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, CHANNEL)
            .setMethodCallHandler { call, result ->
                if (call.method != "request") {
                    result.notImplemented()
                    return@setMethodCallHandler
                }
                // Android 17 (API 37) gates local-network traffic. Older
                // releases have no such permission and need no prompt.
                if (Build.VERSION.SDK_INT < 37) {
                    result.success("notRequired")
                } else if (checkSelfPermission(Manifest.permission.ACCESS_LOCAL_NETWORK) ==
                    PackageManager.PERMISSION_GRANTED
                ) {
                    result.success("granted")
                } else {
                    pending = result
                    requestPermissions(arrayOf(Manifest.permission.ACCESS_LOCAL_NETWORK), REQUEST)
                }
            }
    }

    override fun onRequestPermissionsResult(
        requestCode: Int,
        permissions: Array<out String>,
        grantResults: IntArray,
    ) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode != REQUEST) return
        val result = pending ?: return
        pending = null
        val granted = grantResults.isNotEmpty() &&
            grantResults[0] == PackageManager.PERMISSION_GRANTED
        result.success(if (granted) "granted" else "denied")
    }

    private companion object {
        const val CORE_CHANNEL = "app.parley/core"
        const val CHANNEL = "app.parley/local_network"
        const val REQUEST = 4711
    }
}
