# 🎤 Go gRPC Speech-to-Text with Vosk

## 📌 Overview
This project is a **real-time speech-to-text system** built in **Go** using **gRPC** and **Vosk** for offline speech recognition. It captures **audio from the microphone**, processes it through **Service1**, and transcribes it using **Vosk (Service2)**.

## 📂 Project Structure
```
📁 go-audio-transcription
│── 📁 headunit       # Captures microphone audio & sends it to Service1
│── 📁 service1       # Forwards audio to Service2 for transcription
│── 📁 service2       # Uses Vosk for speech-to-text
│── 📁 proto          # Protobuf definitions for gRPC
│── 📁 vosk-model     # Vosk offline speech recognition model
│── 📄 README.md      # Documentation
```

## 🚀 Features
✅ **Real-time audio streaming** from the microphone.
✅ **gRPC-based communication** between services.
✅ **Offline speech recognition** using **Vosk** (no API key needed).
✅ **Scalable architecture** with separate services.

---

## 🛠️ Setup & Installation

### 1️⃣ Install Dependencies
#### **For macOS (Homebrew)**
```bash
brew install portaudio
brew install wget
```
#### **For Ubuntu/Debian (Linux)**
```bash
sudo apt update && sudo apt install portaudio19-dev wget -y
```
#### **For Windows**
1. Install **PortAudio** from [PortAudio Official Site](http://www.portaudio.com/download.html)
2. Install **Chocolatey** (if not installed) from [Chocolatey.org](https://chocolatey.org/install)
3. Install dependencies using Chocolatey:
```powershell
choco install portaudio wget unzip
```
4. Ensure **Go** is installed: Download it from [golang.org](https://golang.org/dl/)

### 2️⃣ Install Go Dependencies
```bash
go mod tidy
```

### 3️⃣ Download & Set Up Vosk Model
#### **For macOS & Linux**
```bash
wget https://alphacephei.com/vosk/models/vosk-model-small-en-us-0.15.zip
unzip vosk-model-small-en-us-0.15.zip
mv vosk-model-small-en-us-0.15 vosk-model
```
#### **For Windows (PowerShell)**
```powershell
Invoke-WebRequest -Uri "https://alphacephei.com/vosk/models/vosk-model-small-en-us-0.15.zip" -OutFile "vosk-model-small-en-us-0.15.zip"
Expand-Archive -Path "vosk-model-small-en-us-0.15.zip" -DestinationPath "vosk-model"
```

### 4️⃣ Install Python & Vosk Library
```bash
pip install vosk
```

---

## ▶️ Running the Services
### **1️⃣ Start Service2 (Vosk Speech-to-Text)**
```bash
cd service2
go run .
```

### **2️⃣ Start Service1 (Processing Service)**
```bash
cd ../service1
go run .
```

### **3️⃣ Start HeadUnit (Captures Mic Audio & Sends to Service1)**
```bash
cd ../headunit
go run .
```

Now, **speak into the microphone**, and the system will transcribe your speech in real time! 🎤

---

## 📜 Expected Output
```bash
2025/02/06 20:00:00 Recording audio from microphone...
2025/02/06 20:00:10 Audio streaming stopped.
2025/02/06 20:00:12 Final Transcription: "Hey, my name is Khan."
```



## 📜 License
This project is licensed under the **MIT License**.

---

## ⭐ Contributing
Feel free to open **issues & PRs** to improve the project!

