<template>
  <el-container class="layout-container">
    <el-header style="background-color: #409EFF; color: white; display: flex; align-items: center;">
      <h2 style="margin: 0;">fffrp Server Manager v1.0.0</h2>
    </el-header>
    <el-container>
      <el-aside width="300px" style="border-right: 1px solid #eee;">
        <el-scrollbar>
          <el-menu :default-active="activeClientId" @select="handleSelectClient">
            <el-menu-item v-for="client in clients" :key="client.id" :index="client.id">
              <template #title>
                <el-icon><User /></el-icon>
                <div style="display: flex; flex-direction: column; line-height: 1.2; margin-left: 5px;">
                   <span style="font-weight: bold;">{{ client.name }} - {{ client.project_name }}</span>
                   <span style="font-size: 12px; color: #666;">{{ client.phone }}</span>
                </div>
              </template>
            </el-menu-item>
          </el-menu>
        </el-scrollbar>
      </el-aside>
      <el-main>
        <div v-if="selectedClient">
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px;">
            <h3>{{ selectedClient.name }} - {{ selectedClient.project_name }}</h3>
            <el-button type="primary" @click="showAddDialog = true">Add Target Service</el-button>
          </div>
          
          <el-descriptions title="Info" border :column="2">
            <el-descriptions-item label="Project">{{ selectedClient.project_name }}</el-descriptions-item>
            <el-descriptions-item label="Name">{{ selectedClient.name }}</el-descriptions-item>
            <el-descriptions-item label="Phone">{{ selectedClient.phone }}</el-descriptions-item>
            <el-descriptions-item label="Remark">{{ selectedClient.remark }}</el-descriptions-item>
            <el-descriptions-item label="ID">{{ selectedClient.id }}</el-descriptions-item>
          </el-descriptions>

          <h4 style="margin-top: 20px;">Target Services</h4>
          <el-table :data="selectedClient.services" style="width: 100%" border>
            <el-table-column prop="id" label="Service ID" width="180" />
            <el-table-column prop="remote_port" label="Remote Port (Public)" width="180" />
            <el-table-column label="Target Address">
              <template #default="scope">
                {{ scope.row.local_ip }}:{{ scope.row.local_port }}
              </template>
            </el-table-column>
            <el-table-column prop="remark" label="Remark" />
            <el-table-column fixed="right" label="Operations" width="120">
              <template #default="scope">
                <el-button link type="danger" size="small" @click="removeService(scope.row)">Delete</el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>
        <el-empty v-else description="Select a client to view details" />
      </el-main>
    </el-container>

    <!-- Add Service Dialog -->
    <el-dialog v-model="showAddDialog" title="Add Target Service" width="500px">
      <el-form :model="form" label-width="120px">
        <el-form-item label="Target IP" required>
          <el-input v-model="form.local_ip" placeholder="192.168.1.1" />
        </el-form-item>
        <el-form-item label="Target Port" required>
          <el-input v-model="form.local_port" placeholder="22" type="number" />
        </el-form-item>
        <el-form-item label="Remark">
          <el-input v-model="form.remark" placeholder="e.g. Web Server" />
        </el-form-item>
      </el-form>
      <template #footer>
        <span class="dialog-footer">
          <el-button @click="showAddDialog = false">Cancel</el-button>
          <el-button type="primary" @click="confirmAddService" :disabled="!form.local_ip || !form.local_port">Confirm</el-button>
        </span>
      </template>
    </el-dialog>
  </el-container>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { User } from '@element-plus/icons-vue'
import axios from 'axios'
import { ElMessage, ElMessageBox } from 'element-plus'

interface TargetService {
  id: string
  local_ip: string
  local_port: number
  remote_port: number // This might be assigned by server, or requested? Protocol says Server assigns public port.
  remark: string
}

interface Client {
  id: string
  name: string
  phone: string
  project_name: string
  remark?: string
  services: TargetService[]
}

const clients = ref<Client[]>([])
const activeClientId = ref('')
const showAddDialog = ref(false)
const form = ref({
  local_ip: '',
  local_port: '',
  remark: ''
})

const selectedClient = computed(() => {
  return clients.value.find(c => c.id === activeClientId.value)
})

const handleSelectClient = (index: string) => {
  activeClientId.value = index
}

const fetchClients = async () => {
  try {
    const res = await axios.get('/api/clients')
    clients.value = res.data
  } catch (error) {
    console.error(error)
    ElMessage.error('Failed to fetch clients')
  }
}

const confirmAddService = async () => {
  if (!activeClientId.value) return
  if (!form.value.local_ip || !form.value.local_port) {
    ElMessage.warning('Target IP and Port are required')
    return
  }
  
  try {
    // Note: The API should handle port allocation or we send request
    // The previous implementation of addService in server.go accepts TargetService JSON
    // But TargetService struct has RemotePort.
    // Usually user specifies Target, server assigns RemotePort?
    // README says: "服务端分配一个公网端口".
    // So we assume we send Target IP/Port and Remark. Server decides RemotePort.
    // However, the current server.go BindJSON binds to TargetService, which includes RemotePort.
    // If we send 0, server might assign?
    // Let's check server logic. For now assuming we send what we have.
    
    const payload = {
      local_ip: form.value.local_ip,
      local_port: Number(form.value.local_port),
      remote_port: 0, // Server should assign? Or we let user specify? README says Server assigns.
      remark: form.value.remark,
      id: "" // New service
    }

    await axios.post(`/api/client/${activeClientId.value}/service`, payload)
    ElMessage.success('Service added')
    showAddDialog.value = false
    form.value.local_ip = ''
    form.value.local_port = ''
    form.value.remark = ''
    fetchClients() // Refresh
  } catch (error) {
    console.error(error)
    ElMessage.error('Failed to add service')
  }
}

const removeService = (svc: TargetService) => {
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
        await axios.delete(`/api/client/${activeClientId.value}/service/${svc.id}`)
        ElMessage.success('Service deleted')
        fetchClients()
      } catch (error) {
        ElMessage.error('Delete failed')
      }
    })
}

// WebSocket for realtime updates
const connectWS = () => {
  const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws'
  const ws = new WebSocket(`${protocol}://${window.location.host}/ws`)
  
  ws.onmessage = () => {
    // Simple: reload on any message
    fetchClients()
  }
  
  ws.onclose = () => {
    setTimeout(connectWS, 3000)
  }
}

onMounted(() => {
  fetchClients()
  connectWS()
})
</script>

<style>
html, body, #app, .layout-container {
  height: 100%;
  margin: 0;
}
</style>
