import java.util.Properties

plugins {
    id("com.android.application")
    id("kotlin-android")
    // The Flutter Gradle Plugin must be applied after the Android and Kotlin Gradle plugins.
    id("dev.flutter.flutter-gradle-plugin")
}

// Read the same defines as Dart. Disabled builds retain harmless sample
// metadata; the SDK startup provider is removed from the merged manifest.
val flutterProperties = Properties().apply {
    rootProject.file("local.properties").inputStream().use { load(it) }
}
val adConfigProcess = ProcessBuilder(
    "${flutterProperties.getProperty("flutter.sdk")}/bin/cache/dart-sdk/bin/dart",
    "${project.projectDir}/../../tool/native_admob_config.dart", "android",
    if (gradle.startParameter.taskNames.any { it.contains("Release", true) }) "release" else "debug"
).apply {
    environment()["DART_DEFINES"] = project.findProperty("dart-defines")?.toString() ?: ""
    redirectErrorStream(true)
}.start()
val adApplicationId = adConfigProcess.inputStream.bufferedReader().readText().trim()
if (adConfigProcess.waitFor() != 0) throw GradleException("Invalid native AdMob configuration")

android {
    namespace = "com.example.knowoff_client"
    compileSdk = flutter.compileSdkVersion
    ndkVersion = flutter.ndkVersion

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = JavaVersion.VERSION_17.toString()
    }

    defaultConfig {
        manifestPlaceholders["knowoffAdMobAppId"] = adApplicationId
        // TODO: Specify your own unique Application ID (https://developer.android.com/studio/build/application-id.html).
        applicationId = "com.example.knowoff_client"
        // You can update the following values to match your application needs.
        // For more information, see: https://flutter.dev/to/review-gradle-config.
        minSdk = flutter.minSdkVersion
        targetSdk = flutter.targetSdkVersion
        versionCode = flutter.versionCode
        versionName = flutter.versionName
    }

    buildTypes {
        release {
            // TODO: Add your own signing config for the release build.
            // Signing with the debug keys for now, so `flutter run --release` works.
            signingConfig = signingConfigs.getByName("debug")
        }
    }
}

flutter {
    source = "../.."
}
