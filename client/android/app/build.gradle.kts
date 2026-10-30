plugins {
    id("com.android.application")
    // The Flutter Gradle Plugin must be applied after the Android and Kotlin Gradle plugins.
    id("dev.flutter.flutter-gradle-plugin")
}

android {
    namespace = "app.parley.translate"
    compileSdk = 37
    compileSdkMinor = 0
    ndkVersion = flutter.ndkVersion

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    defaultConfig {
        // TODO: Specify your own unique Application ID (https://developer.android.com/studio/build/application-id.html).
        applicationId = "app.parley.translate"
        // You can update the following values to match your application needs.
        // For more information, see: https://flutter.dev/to/review-gradle-config.
        minSdk = 24
        targetSdk = 37
        // Uses the version code from pubspec.yaml. When using split APKs, 1000 * ABI_VERSION
        // is added automatically by Flutter. (https://developer.android.com/studio/build/configure-apk-splits#configure-APK-versions)
        // You can force using the value of versionCode by specifying the `-P force-version-code-ignoring-abi=true`
        // flag during build.
        versionCode = flutter.versionCode
        versionName = flutter.versionName
    }

    // The release workflow exports the four ANDROID_* signing secrets plus
    // ANDROID_KEYSTORE_PATH (decoded outside the repository). Local builds
    // without them stay debug-signed.
    val releaseKeystore = System.getenv("ANDROID_KEYSTORE_PATH")
    signingConfigs {
        if (releaseKeystore != null) {
            create("release") {
                val password = System.getenv("ANDROID_KEYSTORE_PASSWORD")
                val alias = System.getenv("ANDROID_KEY_ALIAS")
                val keyPassword = System.getenv("ANDROID_KEY_PASSWORD")
                require(!password.isNullOrEmpty() && !alias.isNullOrEmpty() && !keyPassword.isNullOrEmpty()) {
                    "ANDROID_KEYSTORE_PATH is set but ANDROID_KEYSTORE_PASSWORD, ANDROID_KEY_ALIAS or ANDROID_KEY_PASSWORD is missing"
                }
                storeFile = file(releaseKeystore)
                storePassword = password
                keyAlias = alias
                this.keyPassword = keyPassword
            }
        }
    }

    buildTypes {
        release {
            signingConfig = if (releaseKeystore != null) {
                signingConfigs.getByName("release")
            } else {
                signingConfigs.getByName("debug")
            }
        }
    }
}

kotlin {
    compilerOptions {
        jvmTarget = org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17
    }
}

flutter {
    source = "../.."
}

dependencies {
    // The gomobile Device Host (Go core for four ABIs). Built by
    // scripts/build-parley-core-android.sh; the AAR is generated, not tracked.
    implementation(files("libs/parleycore.aar"))
}
