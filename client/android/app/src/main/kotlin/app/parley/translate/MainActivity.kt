package app.parley.translate

import android.Manifest
import android.content.pm.PackageManager
import android.os.Build
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel

class MainActivity : FlutterActivity() {
    private var pending: MethodChannel.Result? = null

    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)
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
        const val CHANNEL = "app.parley/local_network"
        const val REQUEST = 4711
    }
}
