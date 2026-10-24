package com.tunmap.app

import android.app.AlertDialog
import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.graphics.Color
import android.os.Bundle
import android.text.InputType
import android.view.Gravity
import android.widget.*
import androidx.appcompat.app.AppCompatActivity
import mobile.Mobile
import mobile.Listener

class MainActivity : AppCompatActivity() {

    private lateinit var portInput: EditText
    private lateinit var bigButton: Button
    private lateinit var result: TextView
    private lateinit var serverLine: TextView
    private var running = false

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        val root = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            gravity = Gravity.CENTER_HORIZONTAL
            setPadding(60, 110, 60, 60)
        }

        root.addView(TextView(this).apply {
            text = "tunmap"
            textSize = 32f
            setTextColor(Color.parseColor("#8b5cf6"))
            gravity = Gravity.CENTER
        })
        root.addView(TextView(this).apply {
            text = "把端口暴露到公网"
            textSize = 14f
            setTextColor(Color.parseColor("#8494ab"))
            setPadding(0, 8, 0, 50)
        })

        root.addView(TextView(this).apply {
            text = "本机要映射的端口"
            textSize = 13f
            setTextColor(Color.parseColor("#8494ab"))
        })
        portInput = EditText(this).apply {
            inputType = InputType.TYPE_CLASS_NUMBER
            setText("5050")
            textSize = 22f
            gravity = Gravity.CENTER
            setTextColor(Color.parseColor("#e8eef7"))
            setBackgroundColor(Color.parseColor("#111826"))
            setPadding(30, 30, 30, 30)
        }
        root.addView(portInput, lp(40))

        bigButton = Button(this).apply {
            text = "启动"
            textSize = 18f
            setTextColor(Color.WHITE)
            setBackgroundColor(Color.parseColor("#8b5cf6"))
        }
        bigButton.setOnClickListener {
            if (running) doStop() else doStart()
        }
        root.addView(bigButton, lp(40))

        result = TextView(this).apply {
            text = ""
            textSize = 14f
            gravity = Gravity.CENTER
            setPadding(0, 46, 0, 0)
        }
        root.addView(result)

        serverLine = TextView(this).apply {
            textSize = 12f
            gravity = Gravity.CENTER
            setTextColor(Color.parseColor("#8494ab"))
            setPadding(0, 20, 0, 0)
            setOnClickListener { showServerDialog() }
        }
        root.addView(serverLine)
        root.addView(TextView(this).apply {
            text = "点这里可修改服务器地址"
            textSize = 11f
            gravity = Gravity.CENTER
            setTextColor(Color.parseColor("#5a6a84"))
            setPadding(0, 4, 0, 0)
        })

        setContentView(root)
        refreshServerLine()
        if (getServer().isEmpty()) showServerDialog()
    }

    private fun lp(top: Int) = LinearLayout.LayoutParams(
        LinearLayout.LayoutParams.MATCH_PARENT,
        LinearLayout.LayoutParams.WRAP_CONTENT
    ).apply { topMargin = top }

    private fun prefs() = getSharedPreferences("tunmap", Context.MODE_PRIVATE)

    private fun getServer(): String = prefs().getString("server", "") ?: ""

    private fun refreshServerLine() {
        val s = getServer()
        serverLine.text = if (s.isEmpty()) "⚠ 还没设置服务器地址（点这里设置）"
        else "服务器：$s"
    }

    private fun showServerDialog() {
        val saved = getServer()
        val input = EditText(this).apply {
            inputType = InputType.TYPE_TEXT_VARIATION_URI
            setText(saved) // 已有值就预填，不再显示占位符
        }
        AlertDialog.Builder(this)
            .setTitle("服务器地址")
            .setMessage("你部署 tunmap 服务端的公网服务器地址，例如 http://123.45.67.89:8080")
            .setView(input)
            .setPositiveButton("保存") { _, _ ->
                val v = input.text.toString().trim().trimEnd('/')
                prefs().edit().putString("server", v).apply()
                refreshServerLine()
                result.setTextColor(Color.parseColor("#8494ab"))
                result.text = "已保存，点启动即可"
            }
            .setNegativeButton("取消", null)
            .show()
    }

    private fun doStart() {
        val port = portInput.text.toString().trim().toLongOrNull()
        if (port == null || port !in 1..65535) {
            showError("端口不对，请填 1-65535 的数字")
            return
        }
        val server = getServer()
        if (server.isEmpty()) {
            showError("还没设置服务器地址")
            showServerDialog()
            return
        }

        bigButton.text = "停止"
        bigButton.setBackgroundColor(Color.parseColor("#f87171"))
        portInput.isEnabled = false
        result.setTextColor(Color.parseColor("#8494ab"))
        result.text = "正在连接服务器…"
        running = true

        Mobile.start(server, "", port, "", object : Listener {
            override fun onReady(publicURL: String) {
                runOnUiThread {
                    result.setTextColor(Color.parseColor("#34d399"))
                    result.text = "成功！公网地址：\n\n$publicURL\n\n（点一下复制）"
                    result.setOnClickListener {
                        val cm = getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
                        cm.setPrimaryClip(ClipData.newPlainText("url", publicURL))
                        Toast.makeText(this@MainActivity, "已复制", Toast.LENGTH_SHORT).show()
                    }
                }
            }
            override fun onLog(line: String) {
                runOnUiThread {
                    val low = line.lowercase()
                    if (low.contains("error") || low.contains("refused") || low.contains("no such host")) {
                        showError("连不上服务器，检查地址\n$line")
                    } else if (line.contains("tunnel ready")) {
                        // waiting for onReady with the public URL
                    } else if (line.contains("已停止")) {
                        if (!running) return@runOnUiThread
                        resetUI("已停止")
                    }
                }
            }
            override fun onError(msg: String) {
                runOnUiThread { showError(msg) }
            }
        })
    }

    private fun doStop() {
        Mobile.stop()
        running = false
        bigButton.text = "启动"
        bigButton.setBackgroundColor(Color.parseColor("#8b5cf6"))
        portInput.isEnabled = true
        result.setTextColor(Color.parseColor("#8494ab"))
        result.text = "已停止"
        result.setOnClickListener(null)
    }

    private fun showError(msg: String) {
        result.setTextColor(Color.parseColor("#f87171"))
        result.text = "失败：$msg"
    }

    private fun resetUI(msg: String) {
        running = false
        bigButton.text = "启动"
        bigButton.setBackgroundColor(Color.parseColor("#8b5cf6"))
        portInput.isEnabled = true
        result.setTextColor(Color.parseColor("#8494ab"))
        result.text = msg
    }
}
