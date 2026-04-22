<script setup>
// Empty state component - no logic needed
</script>

<template>
  <div class="empty-state">
    <!-- Cat avatar -->
    <div class="cat-avatar-container">
      <svg class="cat-avatar" viewBox="0 0 100 100" fill="none" xmlns="http://www.w3.org/2000/svg">
        <!-- Cat face circle -->
        <circle cx="50" cy="55" r="35" fill="#FFB74D"/>
        <!-- Left ear -->
        <path d="M20 35 L30 55 L15 55 Z" fill="#FFB74D"/>
        <path d="M22 40 L28 50 L18 50 Z" fill="#FFCC80"/>
        <!-- Right ear -->
        <path d="M80 35 L70 55 L85 55 Z" fill="#FFB74D"/>
        <path d="M78 40 L72 50 L82 50 Z" fill="#FFCC80"/>
        <!-- Eyes -->
        <ellipse cx="38" cy="50" rx="6" ry="8" fill="#4FC3F7"/>
        <ellipse cx="62" cy="50" rx="6" ry="8" fill="#4FC3F7"/>
        <circle cx="38" cy="48" r="2" fill="#1a1a1a"/>
        <circle cx="62" cy="48" r="2" fill="#1a1a1a"/>
        <!-- Nose -->
        <path d="M50 58 L47 62 L53 62 Z" fill="#FF8A65"/>
        <!-- Mouth -->
        <path d="M50 62 Q45 68 42 65" stroke="#4FC3F7" stroke-width="1.5" fill="none"/>
        <path d="M50 62 Q55 68 58 65" stroke="#4FC3F7" stroke-width="1.5" fill="none"/>
        <!-- Whiskers -->
        <line x1="25" y1="55" x2="10" y2="52" stroke="#4FC3F7" stroke-width="1.5"/>
        <line x1="25" y1="58" x2="10" y2="58" stroke="#4FC3F7" stroke-width="1.5"/>
        <line x1="25" y1="61" x2="10" y2="64" stroke="#4FC3F7" stroke-width="1.5"/>
        <line x1="75" y1="55" x2="90" y2="52" stroke="#4FC3F7" stroke-width="1.5"/>
        <line x1="75" y1="58" x2="90" y2="58" stroke="#4FC3F7" stroke-width="1.5"/>
        <line x1="75" y1="61" x2="90" y2="64" stroke="#4FC3F7" stroke-width="1.5"/>
      </svg>
      <div class="cat-name-badge">小午</div>
    </div>

    <h2>有什么可以帮你的,主人?</h2>
    <p>小午在这里为你服务喵~ 输入问题开始对话,支持文件上传和图片理解</p>
  </div>
</template>

<style scoped>
.cat-avatar-container {
  position: relative;
  display: inline-block;
  margin-bottom: 20px;
}

.cat-avatar {
  width: 100px;
  height: 100px;
}

.cat-name-badge {
  position: absolute;
  bottom: -5px;
  left: 50%;
  transform: translateX(-50%);
  background: linear-gradient(135deg, #FFB74D, #FF8A65);
  color: #fff;
  font-size: 12px;
  font-weight: bold;
  padding: 2px 10px;
  border-radius: 10px;
  box-shadow: 0 2px 8px rgba(0,0,0,0.2);
}
</style>
