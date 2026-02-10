<template>
  <div class="common-layout">
    <el-container>
      <el-header>
        <div style="display: flex; justify-content: space-between; align-items: center; padding: 10px 0;">
          <h2>FFFRP Client v1.0.0</h2>
          <el-tag :type="connected ? 'success' : 'danger'">
            {{ connected ? 'Connected' : 'Disconnected' }}
          </el-tag>
        </div>
      </el-header>
      
      <el-main>
        <!-- Login Form -->
        <div v-if="!isLoggedIn" class="login-container">
          <el-card class="box-card" style="max-width: 480px; margin: 0 auto;">
            <template #header>
              <div class="card-header">
                <span>Connect to Server</span>
              </div>
            </template>
            <el-form :model="loginForm" label-width="120px">
              <el-form-item label="Name">
                <el-input v-model="loginForm.name" placeholder="Enter Name" />
              </el-form-item>
              <el-form-item label="Phone">
                <el-input v-model="loginForm.phone" placeholder="Enter Phone" />
              </el-form-item>
              <el-form-item label="Project Name">
                <el-input v-model="loginForm.projectName" placeholder="Enter Project Name" />
              </el-form-item>
              <el-form-item label="Remark">
                <el-input v-model="loginForm.remark" placeholder="Enter Remark" />
              </el-form-item>
              <el-form-item>
                <el-button type="primary" @click="handleLogin" :loading="loading">Connect</el-button>
              </el-form-item>
            </el-form>
          </el-card>
        </div>

        <!-- Main Interface -->
        <div v-else>
          <div v-if="status" style="margin-bottom: 20px; display: flex; justify-content: space-between; align-items: center;">
             <div style="display: flex; align-items: center; gap: 10px;">
               <span style="font-weight: bold;">Status:</span>
               <el-tag :type="status.connected ? 'success' : 'danger'" effect="dark">
                 {{ status.connected ? 'Connected' : 'Disconnected' }}
               </el-tag>
             </div>
             <el-button type="primary" @click="dialogVisible = true">Add Target Service</el-button>
          </div>

          <el-dialog v-model="dialogVisible" title="Add Target Service" width="400px">
            <el-form :model="form" label-width="100px">
              <el-form-item label="Target IP" required>
                <el-input v-model="form.local_ip" placeholder="192.168.1.1" />
              </el-form-item>
              <el-form-item label="Target Port" required>
                <el-input v-model="form.local_port" placeholder="22" type="number" />
              </el-form-item>
              <el-form-item label="Remark">
                <el-input v-model="form.remark" placeholder="Web Server" />
              </el-form-item>
            </el-form>
            <template #footer>
              <span class="dialog-footer">
                <el-button @click="dialogVisible = false">Cancel</el-button>
                <el-button type="primary" @click="onSubmit" :disabled="!form.local_ip || !form.local_port">Add</el-button>
              </span>
            </template>
          </el-dialog>

          <el-divider />

          <el-table :data="services" style="width: 100%">
            <el-table-column prop="local_ip" label="Target IP" width="180" />
            <el-table-column prop="local_port" label="Target Port" />
            <el-table-column prop="remark" label="Remark" />
            <el-table-column fixed="right" label="Operations" width="120">
              <template #default="scope">
                <el-button link type="danger" size="small" @click="removeService(scope.row)">Delete</el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>
      </el-main>
    </el-container>
  </div>
</template>

<script lang="ts" setup>
import { ref, onMounted } from 'vue'
import { GetStatus, AddTarget, Login, RemoveTarget } from '../wailsjs/go/main/App'
import { EventsOn } from '../wailsjs/runtime/runtime'
import { ElMessage, ElMessageBox } from 'element-plus'

const isLoggedIn = ref(false)
const connected = ref(false)
const loading = ref(false)
const dialogVisible = ref(false)
const status = ref<any>(null)
const services = ref<any[]>([])

const loginForm = ref({
  name: '',
  phone: '',
  projectName: '',
  remark: ''
})

const form = ref({
  local_ip: '',
  local_port: '',
  remote_port: 0,
  remark: ''
})

const handleLogin = async () => {
  if (!loginForm.value.name || !loginForm.value.phone || !loginForm.value.projectName) {
    alert("Please fill in all required fields (Name, Phone, Project Name)")
    return
  }
  
  loading.value = true
  try {
    await Login(loginForm.value.name, loginForm.value.phone, loginForm.value.projectName, loginForm.value.remark)
    isLoggedIn.value = true
    updateStatus()
  } catch (e) {
    alert("Connection Failed: " + e)
  } finally {
    loading.value = false
  }
}

const updateStatus = async () => {
  if (!isLoggedIn.value) return
  try {
    const s = await GetStatus()
    console.log("Status:", s)
    status.value = s
    connected.value = s.connected
    services.value = s.services || []
  } catch (e) {
    console.error(e)
  }
}

const onSubmit = async () => {
  if (!form.value.local_ip || !form.value.local_port) {
    // Should be disabled but double check
    return
  }
  try {
    // remote_port is always 0 (Auto)
    await AddTarget(form.value.local_ip, Number(form.value.local_port), 0, form.value.remark)
    dialogVisible.value = false
    form.value.local_ip = ''
    form.value.local_port = ''
    form.value.remark = ''
    updateStatus()
  } catch (e) {
    alert("Error: " + e)
  }
}

const removeService = (svc: any) => {
  ElMessageBox.confirm(
    `Are you sure to delete service mapping to ${svc.local_ip}:${svc.local_port}?`,
    'Warning',
    {
      confirmButtonText: 'OK',
      cancelButtonText: 'Cancel',
      type: 'warning',
    }
  )
    .then(async () => {
      try {
        await RemoveTarget(svc.id)
        ElMessage.success('Service deleted')
        updateStatus()
      } catch (e) {
        ElMessage.error('Delete failed: ' + e)
      }
    })
}

onMounted(() => {
  // Check if already connected/logged in? 
  // For now, assume fresh start requires login.
  // But if we reload page, we might check status.
  checkInitialStatus()

  setInterval(updateStatus, 2000)

  EventsOn("connection-state", (data: any) => {
    console.log("Connection State:", data)
    updateStatus()
  })
  
  EventsOn("state-update", (data: any) => {
    console.log("State Update:", data)
    // Update local state from event data directly
    status.value = data
    connected.value = data.connected
    services.value = data.services || []
  })
})

const checkInitialStatus = async () => {
  try {
    const s = await GetStatus()
    
    // Autofill user info if available
    if (s.user) {
        loginForm.value.name = s.user.name || ''
        loginForm.value.phone = s.user.phone || ''
        loginForm.value.projectName = s.user.project_name || ''
        loginForm.value.remark = s.user.remark || ''
    }

    if (s.connected) {
        // If already connected (e.g. page reload but backend alive), skip login
        // But we don't know if we have user info. 
        // Assuming if connected, we are good.
        isLoggedIn.value = true
        status.value = s
        connected.value = s.connected
        services.value = s.services || []
    } else if (s.user && s.user.name && s.user.phone && s.user.project_name) {
        // Auto login if we have saved credentials
        // But maybe user wants to change?
        // Let's just prefill for now.
        // Or we can try auto-connect?
        // "省的每次启动 都要填" -> Prefill is good.
    }
  } catch (e) {
      // Ignore
  }
}
</script>

<style>
html, body, #app {
  height: 100%;
  margin: 0;
  padding: 0;
}
.common-layout {
  height: 100%;
}
.el-container {
  height: 100%;
}
.el-header {
  background-color: #f5f7fa;
  color: #333;
  line-height: 60px;
  border-bottom: 1px solid #e6e6e6;
}
.login-container {
    display: flex;
    justify-content: center;
    align-items: center;
    height: 100%;
    background-color: #f0f2f5;
}
</style>
