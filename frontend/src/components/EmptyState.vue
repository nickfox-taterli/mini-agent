<script setup>
// Empty state component - no logic needed
</script>

<template>
  <div class="empty-state">
    <svg class="empty-state-icon" viewBox="0 0 120 120" fill="none" xmlns="http://www.w3.org/2000/svg">
      <defs>
        <linearGradient id="brainGrad" x1="0%" y1="0%" x2="100%" y2="100%">
          <stop offset="0%" style="stop-color:#4FC3F7;stop-opacity:1" />
          <stop offset="100%" style="stop-color:#29B6F6;stop-opacity:1" />
        </linearGradient>
        <linearGradient id="glowGrad" x1="0%" y1="0%" x2="100%" y2="100%">
          <stop offset="0%" style="stop-color:#4FC3F7;stop-opacity:0.3" />
          <stop offset="100%" style="stop-color:#29B6F6;stop-opacity:0.1" />
        </linearGradient>
      </defs>
      <!-- Outer glow circle -->
      <circle cx="60" cy="60" r="55" fill="url(#glowGrad)" opacity="0.5"/>
      <!-- Main brain shape -->
      <g transform="translate(60, 55)">
        <!-- Brain outline -->
        <path d="M-25 -20 C-35 -20 -40 -10 -38 0 C-42 5 -40 15 -35 20 C-38 28 -35 35 -28 38 C-25 42 -15 45 -5 42 C0 48 10 48 18 42 C28 45 35 40 38 32 C45 30 50 22 48 12 C52 5 50 -8 42 -15 C40 -28 28 -35 15 -32 C8 -38 -5 -38 -15 -32 C-25 -35 -32 -28 -30 -18 Z"
              fill="#252526" stroke="url(#brainGrad)" stroke-width="2"/>
        <!-- Neural connections -->
        <g stroke="url(#brainGrad)" stroke-width="1.5" fill="none" opacity="0.8">
          <path d="M-15 -15 Q0 -5 15 -15"/>
          <path d="M-20 0 Q0 10 20 0"/>
          <path d="M-15 15 Q0 5 15 15"/>
          <path d="M-8 -15 Q-8 0 -8 15"/>
          <path d="M8 -15 Q8 0 8 15"/>
        </g>
        <!-- Glowing nodes -->
        <circle cx="-15" cy="-15" r="4" fill="#4FC3F7" opacity="0.9"/>
        <circle cx="15" cy="-15" r="4" fill="#4FC3F7" opacity="0.9"/>
        <circle cx="-20" cy="0" r="3" fill="#4FC3F7" opacity="0.7"/>
        <circle cx="20" cy="0" r="3" fill="#4FC3F7" opacity="0.7"/>
        <circle cx="-15" cy="15" r="3" fill="#4FC3F7" opacity="0.7"/>
        <circle cx="15" cy="15" r="3" fill="#4FC3F7" opacity="0.7"/>
        <circle cx="0" cy="0" r="5" fill="#4FC3F7" opacity="0.6"/>
      </g>
      <!-- Chat bubble -->
      <path d="M30 75 L25 90 L45 85 C50 95 65 95 70 85 L90 90 L85 75 C95 70 95 55 85 50 L70 45 C70 45 50 40 45 50 L30 50 C20 55 20 70 30 75 Z"
            fill="#252526" stroke="url(#brainGrad)" stroke-width="2"/>
      <!-- Chat dots -->
      <circle cx="52" cy="62" r="3" fill="#4FC3F7"/>
      <circle cx="60" cy="62" r="3" fill="#4FC3F7"/>
      <circle cx="68" cy="62" r="3" fill="#4FC3F7"/>
      <!-- Sparkles -->
      <g fill="#4FC3F7" opacity="0.6">
        <circle cx="20" cy="30" r="2"/>
        <circle cx="100" cy="40" r="2"/>
        <circle cx="95" cy="90" r="1.5"/>
        <circle cx="25" cy="95" r="1.5"/>
      </g>
    </svg>
    <h2>有什么可以帮你的?</h2>
    <p>输入问题开始对话,支持文件上传和图片理解</p>
  </div>
</template>
