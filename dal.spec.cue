package dalforge

// ===================================================
// dal.spec.cue — dalforge-hub 핵심 스펙
// 이 파일은 모든 dal 구성요소의 규약을 정의한다.
// 변경 시 하위 호환성을 반드시 유지해야 한다.
// ===================================================

// ===== 스펙 버전 =====

#SpecVersion: =~"^[0-9]+\\.[0-9]+\\.[0-9]+$"

// 하위 호환 정책:
//   major 변경: 기존 .dalfactory 호환 깨짐 (마이그레이션 필수)
//   minor 변경: 필드 추가만 허용 (기존 유효성 유지)
//   patch 변경: 설명/주석만 변경

// ===== DAL ID 체계 =====

// 형식: DAL:{CATEGORY}:{uuid8}
// uuid8은 최초 발급 후 영구 고정, 재사용 금지
#DalID: =~"^DAL:[A-Z][A-Z0-9_]+:[a-f0-9]{8}$"

// 카테고리는 열거형이 아닌 패턴으로 정의
// 새 카테고리 추가 시 spec 변경 없이 dalcenter에서 등록
#CategoryID: =~"^[A-Z][A-Z0-9_]+$"

// 기본 카테고리 (dalcenter 초기화 시 반드시 존재)
#BuiltinCategory: {
	id!:          #CategoryID
	description!: string & != ""
	name_prefix!: string & =~"^dal[a-z]+-$"
}

builtin_categories: [Name=string]: #BuiltinCategory & {id: Name}
builtin_categories: {
	CLI: {
		id:          "CLI"
		description: "명령줄 도구"
		name_prefix: "dalcli-"
	}
	PLAYER: {
		id:          "PLAYER"
		description: "실행 환경"
		name_prefix: "dalplayer-"
	}
	CONTAINER: {
		id:          "CONTAINER"
		description: "컨테이너 서비스"
		name_prefix: "dalcontainer-"
	}
	SKILL: {
		id:          "SKILL"
		description: "에이전트 스킬"
		name_prefix: "dalskill-"
	}
	HOOK: {
		id:          "HOOK"
		description: "이벤트 훅"
		name_prefix: "dalhook-"
	}
}

// ===== 패키지 메타데이터 =====

#PackageStatus: "active" | "deprecated" | "retired"

#Package: {
	id!:          #DalID
	uuid!:        string & =~"^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$"
	name!:        string & != ""
	category!:    #CategoryID
	version!:     #SpecVersion
	description!: string & != ""
	status!:      #PackageStatus
	aliases?: [...string]
	retired_by?: #DalID
	created_at!: string
	updated_at?: string
}

// ===== 의존성 =====

#Dependency: {
	id!:       #DalID
	version?:  string
	optional?: bool | *false
}

// ===== 빌드 스펙 =====

#BuildSpec: {
	language!: string & != ""
	entry!:    string & != ""
	output!:   string & != ""
	scripts?: {
		pre_build?:    string
		post_build?:   string
		pre_install?:  string
		post_install?: string
	}
}

// ===== .dalfactory (인형 설계도) =====

// 각 레포 루트에 위치하며, localdal 생성의 기반이 된다.
#DalFactory: {
	schema_version!: #SpecVersion
	dal!: {
		id!:       #DalID
		name!:     string & != ""
		version!:  #SpecVersion
		category!: #CategoryID
	}
	description?: string
	depends?: [...#Dependency]
	cli?: [...#DalID]
	skills?: [...#DalID]
	hooks?: [...#DalID]
	agents?: [...string]
	build?: #BuildSpec
}

// ===== localdal (인형 인스턴스) =====

// localdal 하나 = dal 인형 하나
// .dalfactory 기반으로 생성되며, 내부에 CLI/스킬/훅을 담는다.
#LocalDalStatus: "active" | "stopped" | "error" | "updating"

#LocalDal: {
	dal_id!:    #DalID
	node_id!:   string & != ""
	factory!:   string & != ""
	status!:    #LocalDalStatus
	installed!: [...#InstalledPackage]
	created_at!: string
	updated_at!: string
}

#InstalledPackage: {
	id!:           #DalID
	version!:      #SpecVersion
	installed_at!: string
	path!:         string & != ""
}

// ===== dalcenter 레지스트리 =====

#Registry: {
	schema_version!: #SpecVersion
	packages!: [string]: #Package
	categories!: [string]: #BuiltinCategory
}

// ===== dalcenter 노드 인벤토리 =====

#NodeInventory: {
	node_id!:   string & != ""
	hostname?:  string
	dals!:      [...#LocalDal]
	last_sync!: string
}

// ===== 감사 이벤트 =====

#AuditAction: "summon" | "install" | "update" | "remove" | "deprecate" | "retire"

#AuditEvent: {
	id!:        string & != ""
	dal_id!:    #DalID
	action!:    #AuditAction
	actor?:     string
	node_id?:   string
	detail?:    string
	timestamp!: string
}
