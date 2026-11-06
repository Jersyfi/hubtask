// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package eu.hubtask.app

import android.app.Activity
import android.content.Context
import android.content.Intent
import android.graphics.Color
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import androidx.core.view.WindowCompat
import app.tauri.annotation.Command
import app.tauri.annotation.InvokeArg
import app.tauri.annotation.TauriPlugin
import app.tauri.plugin.Invoke
import app.tauri.plugin.JSObject
import app.tauri.plugin.Plugin
import java.net.URL
import java.security.KeyStore
import javax.net.ssl.HttpsURLConnection
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

@InvokeArg
class SecretArgs {
  lateinit var service: String
  lateinit var name: String
  var value: String? = null
}

@InvokeArg
class ShareArgs {
  lateinit var title: String
  lateinit var url: String
}

@InvokeArg
class FetchArgs {
  lateinit var url: String
}

@InvokeArg
class BarsArgs {
  lateinit var background: String
  var isDark: String = "false"
}

/**
 * The Android app's own bridge (F7 Decision 4), called from the shell's Rust (`android.rs`):
 *
 * - The keystore (ADR-0033 §4): an entry - a copy's key, a session - is sealed with AES-GCM under a
 *   key the Android Keystore holds and never hands out, and kept sealed in the app's private
 *   preferences. Android removes both with the app.
 * - The system's share sheet for an entry's title and link (UC-SYN-07 check 8).
 * - The system bars in the page's background, with icons that read on it.
 * - The release's update manifest as text (UC-SYN-07 check 6), which the page may not fetch.
 */
@TauriPlugin
class HubtaskPlugin(private val activity: Activity) : Plugin(activity) {
  private val store = activity.getSharedPreferences("eu.hubtask.app.keystore", Context.MODE_PRIVATE)

  private fun entry(args: SecretArgs) = "${args.service}/${args.name}"

  private fun sealingKey(): SecretKey {
    val keystore = KeyStore.getInstance(KEYSTORE).apply { load(null) }
    (keystore.getKey(ALIAS, null) as SecretKey?)?.let { return it }
    val generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, KEYSTORE)
    generator.init(
      KeyGenParameterSpec.Builder(ALIAS, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
        .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
        .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
        .setKeySize(256)
        .build(),
    )
    return generator.generateKey()
  }

  @Command
  fun secretRead(invoke: Invoke) {
    try {
      val args = invoke.parseArgs(SecretArgs::class.java)
      val answer = JSObject()
      store.getString(entry(args), null)?.let { kept ->
        val sealed = Base64.decode(kept, Base64.NO_WRAP)
        val cipher = Cipher.getInstance(TRANSFORMATION)
        cipher.init(Cipher.DECRYPT_MODE, sealingKey(), GCMParameterSpec(TAG_BITS, sealed, 0, IV_BYTES))
        answer.put("value", String(cipher.doFinal(sealed, IV_BYTES, sealed.size - IV_BYTES), Charsets.UTF_8))
      }
      invoke.resolve(answer)
    } catch (error: Exception) {
      invoke.reject("copy.keystore_unavailable")
    }
  }

  @Command
  fun secretWrite(invoke: Invoke) {
    try {
      val args = invoke.parseArgs(SecretArgs::class.java)
      val cipher = Cipher.getInstance(TRANSFORMATION)
      cipher.init(Cipher.ENCRYPT_MODE, sealingKey())
      val sealed = cipher.iv + cipher.doFinal((args.value ?: "").toByteArray(Charsets.UTF_8))
      check(cipher.iv.size == IV_BYTES)
      store.edit().putString(entry(args), Base64.encodeToString(sealed, Base64.NO_WRAP)).commit()
      invoke.resolve(JSObject())
    } catch (error: Exception) {
      invoke.reject("copy.keystore_unavailable")
    }
  }

  @Command
  fun secretRemove(invoke: Invoke) {
    val args = invoke.parseArgs(SecretArgs::class.java)
    store.edit().remove(entry(args)).commit()
    invoke.resolve(JSObject())
  }

  @Command
  fun share(invoke: Invoke) {
    val args = invoke.parseArgs(ShareArgs::class.java)
    // The title goes with the link, so the receiver sees what it is (the approved preview).
    val send = Intent(Intent.ACTION_SEND).apply {
      type = "text/plain"
      putExtra(Intent.EXTRA_TEXT, "${args.title}\n${args.url}")
      putExtra(Intent.EXTRA_SUBJECT, args.title)
      putExtra(Intent.EXTRA_TITLE, args.title)
    }
    activity.runOnUiThread { activity.startActivity(Intent.createChooser(send, null)) }
    invoke.resolve(JSObject())
  }

  @Command
  fun bars(invoke: Invoke) {
    val args = invoke.parseArgs(BarsArgs::class.java)
    val colour = try {
      Color.parseColor(args.background)
    } catch (error: IllegalArgumentException) {
      invoke.reject("shell.bars_unavailable")
      return
    }
    val isDark = args.isDark == "true"
    activity.runOnUiThread {
      // Edge to edge: the bars are transparent, and what shows through them is the window's own
      // background, which `MainActivity` keeps clear of the page.
      activity.window.decorView.setBackgroundColor(colour)
      WindowCompat.getInsetsController(activity.window, activity.window.decorView).apply {
        isAppearanceLightStatusBars = !isDark
        isAppearanceLightNavigationBars = !isDark
      }
    }
    invoke.resolve(JSObject())
  }

  @Command
  fun fetchText(invoke: Invoke) {
    val args = invoke.parseArgs(FetchArgs::class.java)
    // Off the main thread, which Android keeps free of the network; bounded, as every call is.
    Thread {
      try {
        val url = URL(args.url)
        check(url.protocol == "https")
        val connection = url.openConnection() as HttpsURLConnection
        connection.connectTimeout = TIMEOUT_MS
        connection.readTimeout = TIMEOUT_MS
        try {
          check(connection.responseCode == 200)
          val text = connection.inputStream.use { it.readBytes() }
          check(text.size <= MAX_TEXT)
          invoke.resolve(JSObject().apply { put("text", String(text, Charsets.UTF_8)) })
        } finally {
          connection.disconnect()
        }
      } catch (error: Exception) {
        invoke.reject("update.unavailable")
      }
    }.start()
  }

  private companion object {
    const val TIMEOUT_MS = 30_000
    const val MAX_TEXT = 1 shl 20
    const val KEYSTORE = "AndroidKeyStore"
    const val ALIAS = "eu.hubtask.app.seal"
    const val TRANSFORMATION = "AES/GCM/NoPadding"
    const val IV_BYTES = 12
    const val TAG_BITS = 128
  }
}
