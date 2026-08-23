import React from 'react'
import { Github, Heart, MessageSquare } from 'lucide-react'
import logoImg from '../assets/DockerCopilot-logo.png'

export function About() {
  return (
    <div className="w-full">

      <div className="px-2 sm:px-6 py-4 pt-4 sm:pt-4 space-y-6">
        {/* 项目展示卡片 */}
        <div className="card p-8 flex flex-col items-center text-center relative overflow-hidden">
          <div className="relative mb-4">
            <div className="absolute inset-0 bg-primary-400/20 blur-xl rounded-full"></div>
            <img
              src={logoImg}
              alt="Docker Copilot"
              className="relative w-20 h-20 rounded-2xl shadow-lg"
            />
          </div>

          <h1 className="text-3xl font-bold text-gray-900 dark:text-white mb-2">Docker Copilot</h1>
          <p className="text-gray-600 dark:text-gray-400 max-w-lg mx-auto mb-6">
            一个简洁、优雅且强大的 Docker 容器管理工具，旨在为您提供流畅的容器运维体验。
          </p>

          <div className="flex flex-wrap items-center justify-center gap-3">
            <a
              href="https://github.com/l429609201/dockerCopilot"
              target="_blank"
              rel="noopener noreferrer"
              className="flex items-center gap-2 px-4 py-2 bg-gray-900 text-white rounded-lg hover:bg-gray-800 transition-colors shadow-sm"
            >
              <Github className="h-4 w-4" />
              <span>本项目 GitHub</span>
            </a>
            <a
              href="https://github.com/onlyLTY/dockerCopilot"
              target="_blank"
              rel="noopener noreferrer"
              className="flex items-center gap-2 px-4 py-2 bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300 rounded-lg hover:bg-blue-200 dark:hover:bg-blue-900/50 transition-colors"
            >
              <Github className="h-4 w-4" />
              <span>原项目 (onlyLTY)</span>
            </a>
          </div>
        </div>

        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          {/* 致谢 */}
          <div className="card p-6 flex flex-col h-full">
            <div className="flex items-center gap-2 mb-4">
              <Heart className="h-5 w-5 text-red-500" />
              <h3 className="text-lg font-bold text-gray-900 dark:text-white">致谢 / Thanks</h3>
            </div>
            <p className="text-gray-600 dark:text-gray-400 leading-relaxed flex-1">
              本项目 fork 自 <span className="font-medium">onlyLTY/dockerCopilot</span>，衷心感谢原作者 onlyLTY 及所有社区贡献者的付出——没有原项目就没有本 Fork。本 Fork 在其基础上新增了容器文件管理器、任务中心、镜像图标自适应等功能，并继续以 AGPLv3 开源。
            </p>
          </div>

          {/* 反馈 */}
          <div className="card p-6 flex flex-col h-full">
            <div className="flex items-center gap-2 mb-4">
              <MessageSquare className="h-5 w-5 text-green-500" />
              <h3 className="text-lg font-bold text-gray-900 dark:text-white">反馈与建议</h3>
            </div>
            <p className="text-gray-600 dark:text-gray-400 leading-relaxed mb-4">
              本项目是 fork 版本，在项目使用中遇到 Bug 或有新的功能想法？欢迎到本 Fork 仓库提交 Issue。您的每一个反馈都至关重要。
            </p>
            <a
              href="https://github.com/l429609201/dockerCopilot/issues"
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center text-primary-600 dark:text-primary-400 hover:underline font-medium"
            >
              前往 GitHub Issues &rarr;
            </a>
          </div>
        </div>
      </div>
    </div>
  )
}