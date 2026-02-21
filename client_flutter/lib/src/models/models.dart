class AuthSession {
  AuthSession({
    required this.baseUrl,
    required this.accessToken,
    required this.refreshToken,
    required this.accessTokenExpiresAt,
  });

  final String baseUrl;
  final String accessToken;
  final String refreshToken;
  final DateTime accessTokenExpiresAt;
}

class ExperimentRecord {
  ExperimentRecord({
    required this.localId,
    required this.serverId,
    required this.title,
    required this.status,
    required this.effectiveBody,
    required this.originalEntryServerId,
    required this.effectiveEntryServerId,
    required this.updatedAt,
  });

  final String localId;
  final String? serverId;
  final String title;
  final String status;
  final String effectiveBody;
  final String? originalEntryServerId;
  final String? effectiveEntryServerId;
  final DateTime updatedAt;
}

class EntryRecord {
  EntryRecord({
    required this.localId,
    required this.experimentLocalId,
    required this.serverId,
    required this.entryType,
    required this.supersedesServerId,
    required this.body,
    required this.createdAt,
  });

  final String localId;
  final String experimentLocalId;
  final String? serverId;
  final String entryType;
  final String? supersedesServerId;
  final String body;
  final DateTime createdAt;
}

class CommentRecord {
  CommentRecord({
    required this.localId,
    required this.experimentLocalId,
    required this.serverId,
    required this.body,
    required this.createdAt,
  });

  final String localId;
  final String experimentLocalId;
  final String? serverId;
  final String body;
  final DateTime createdAt;
}

class ProposalRecord {
  ProposalRecord({
    required this.localId,
    required this.sourceExperimentLocalId,
    required this.serverId,
    required this.title,
    required this.body,
    required this.createdAt,
  });

  final String localId;
  final String sourceExperimentLocalId;
  final String? serverId;
  final String title;
  final String body;
  final DateTime createdAt;
}

class OutboxItem {
  OutboxItem({
    required this.id,
    required this.mutationType,
    required this.payloadJson,
    required this.status,
    required this.attempts,
  });

  final int id;
  final String mutationType;
  final String payloadJson;
  final String status;
  final int attempts;
}

class SyncEvent {
  SyncEvent({
    required this.cursor,
    required this.eventType,
    required this.aggregateType,
    required this.aggregateId,
    required this.payload,
  });

  final int cursor;
  final String eventType;
  final String aggregateType;
  final String? aggregateId;
  final Map<String, dynamic> payload;

  factory SyncEvent.fromJson(Map<String, dynamic> json) {
    return SyncEvent(
      cursor: (json['cursor'] as num).toInt(),
      eventType: json['eventType'] as String,
      aggregateType: json['aggregateType'] as String,
      aggregateId: json['aggregateId'] as String?,
      payload: (json['payload'] as Map<String, dynamic>? ?? <String, dynamic>{}),
    );
  }
}

class ConflictArtifact {
  ConflictArtifact({
    required this.conflictArtifactId,
    required this.experimentId,
    required this.clientBaseEntryId,
    required this.serverLatestEntryId,
    required this.createdAt,
  });

  final String conflictArtifactId;
  final String experimentId;
  final String? clientBaseEntryId;
  final String? serverLatestEntryId;
  final DateTime createdAt;

  factory ConflictArtifact.fromJson(Map<String, dynamic> json) {
    return ConflictArtifact(
      conflictArtifactId: json['conflictArtifactId'] as String,
      experimentId: json['experimentId'] as String,
      clientBaseEntryId: json['clientBaseEntryId'] as String?,
      serverLatestEntryId: json['serverLatestEntryId'] as String?,
      createdAt: DateTime.parse(json['createdAt'] as String),
    );
  }
}

// ---------------------------------------------------------------------------
// Protocol models
// ---------------------------------------------------------------------------

class ProtocolRecord {
  ProtocolRecord({
    required this.protocolId,
    required this.creatorUserId,
    required this.title,
    required this.description,
    required this.status,
    required this.createdAt,
  });

  final String protocolId;
  final String creatorUserId;
  final String title;
  final String description;
  final String status;
  final DateTime createdAt;

  factory ProtocolRecord.fromJson(Map<String, dynamic> json) {
    return ProtocolRecord(
      protocolId: json['protocolId'] as String,
      creatorUserId: json['creatorUserId'] as String? ?? '',
      title: json['title'] as String,
      description: json['description'] as String? ?? '',
      status: json['status'] as String? ?? 'draft',
      createdAt: DateTime.parse(json['createdAt'] as String),
    );
  }
}

class ProtocolVersionRecord {
  ProtocolVersionRecord({
    required this.versionNum,
    required this.authorUserId,
    required this.body,
    required this.changeLog,
    required this.publishedAt,
  });

  final int versionNum;
  final String authorUserId;
  final String body;
  final String changeLog;
  final DateTime publishedAt;

  factory ProtocolVersionRecord.fromJson(Map<String, dynamic> json) {
    return ProtocolVersionRecord(
      versionNum: (json['versionNum'] as num).toInt(),
      authorUserId: json['authorUserId'] as String? ?? '',
      body: json['body'] as String,
      changeLog: json['changeLog'] as String? ?? '',
      publishedAt: DateTime.parse(json['publishedAt'] as String),
    );
  }
}

class DeviationRecord {
  DeviationRecord({
    required this.deviationId,
    required this.experimentId,
    required this.protocolId,
    required this.reportedBy,
    required this.description,
    required this.severity,
    required this.createdAt,
  });

  final String deviationId;
  final String experimentId;
  final String protocolId;
  final String reportedBy;
  final String description;
  final String severity;
  final DateTime createdAt;

  factory DeviationRecord.fromJson(Map<String, dynamic> json) {
    return DeviationRecord(
      deviationId: json['deviationId'] as String,
      experimentId: json['experimentId'] as String,
      protocolId: json['protocolId'] as String,
      reportedBy: json['reportedBy'] as String? ?? '',
      description: json['description'] as String,
      severity: json['severity'] as String? ?? 'minor',
      createdAt: DateTime.parse(json['createdAt'] as String),
    );
  }
}

// ---------------------------------------------------------------------------
// Signature models
// ---------------------------------------------------------------------------

class SignatureRecord {
  SignatureRecord({
    required this.signatureId,
    required this.experimentId,
    required this.signerUserId,
    required this.signerEmail,
    required this.role,
    required this.meaning,
    required this.contentHash,
    required this.signedAt,
  });

  final String signatureId;
  final String experimentId;
  final String signerUserId;
  final String signerEmail;
  final String role;
  final String meaning;
  final String contentHash;
  final DateTime signedAt;

  factory SignatureRecord.fromJson(Map<String, dynamic> json) {
    return SignatureRecord(
      signatureId: json['signatureId'] as String,
      experimentId: json['experimentId'] as String,
      signerUserId: json['signerUserId'] as String? ?? '',
      signerEmail: json['signerEmail'] as String? ?? '',
      role: json['role'] as String? ?? 'author',
      meaning: json['meaning'] as String? ?? '',
      contentHash: json['contentHash'] as String? ?? '',
      signedAt: DateTime.parse(json['signedAt'] as String),
    );
  }
}

// ---------------------------------------------------------------------------
// Notification models
// ---------------------------------------------------------------------------

class NotificationRecord {
  NotificationRecord({
    required this.notificationId,
    required this.userId,
    required this.eventType,
    required this.title,
    required this.body,
    this.experimentId,
    required this.isRead,
    required this.createdAt,
  });

  final String notificationId;
  final String userId;
  final String eventType;
  final String title;
  final String body;
  final String? experimentId;
  final bool isRead;
  final DateTime createdAt;

  factory NotificationRecord.fromJson(Map<String, dynamic> json) {
    return NotificationRecord(
      notificationId: json['notificationId'] as String,
      userId: json['userId'] as String? ?? '',
      eventType: json['eventType'] as String? ?? '',
      title: json['title'] as String? ?? '',
      body: json['body'] as String? ?? '',
      experimentId: json['experimentId'] as String?,
      isRead: json['isRead'] as bool? ?? false,
      createdAt: DateTime.parse(json['createdAt'] as String),
    );
  }
}

// ---------------------------------------------------------------------------
// Data visualization models
// ---------------------------------------------------------------------------

class DataExtractRecord {
  DataExtractRecord({
    required this.dataExtractId,
    required this.attachmentId,
    required this.experimentId,
    required this.columnHeaders,
    required this.rowCount,
    required this.sampleRows,
    required this.parsedAt,
  });

  final String dataExtractId;
  final String attachmentId;
  final String experimentId;
  final List<String> columnHeaders;
  final int rowCount;
  final List<List<String>> sampleRows;
  final DateTime parsedAt;

  factory DataExtractRecord.fromJson(Map<String, dynamic> json) {
    return DataExtractRecord(
      dataExtractId: json['dataExtractId'] as String,
      attachmentId: json['attachmentId'] as String? ?? '',
      experimentId: json['experimentId'] as String? ?? '',
      columnHeaders: (json['columnHeaders'] as List<dynamic>? ?? <dynamic>[])
          .cast<String>(),
      rowCount: (json['rowCount'] as num?)?.toInt() ?? 0,
      sampleRows: (json['sampleRows'] as List<dynamic>? ?? <dynamic>[])
          .map((row) => (row as List<dynamic>).cast<String>())
          .toList(),
      parsedAt: DateTime.parse(json['parsedAt'] as String),
    );
  }
}

class ChartConfigRecord {
  ChartConfigRecord({
    required this.chartConfigId,
    required this.experimentId,
    required this.dataExtractId,
    required this.chartType,
    required this.title,
    required this.xColumn,
    required this.yColumns,
    required this.options,
    required this.createdAt,
  });

  final String chartConfigId;
  final String experimentId;
  final String dataExtractId;
  final String chartType;
  final String title;
  final String xColumn;
  final List<String> yColumns;
  final Map<String, dynamic> options;
  final DateTime createdAt;

  factory ChartConfigRecord.fromJson(Map<String, dynamic> json) {
    return ChartConfigRecord(
      chartConfigId: json['chartConfigId'] as String,
      experimentId: json['experimentId'] as String? ?? '',
      dataExtractId: json['dataExtractId'] as String? ?? '',
      chartType: json['chartType'] as String? ?? 'line',
      title: json['title'] as String? ?? '',
      xColumn: json['xColumn'] as String? ?? '',
      yColumns: (json['yColumns'] as List<dynamic>? ?? <dynamic>[])
          .cast<String>(),
      options: json['options'] as Map<String, dynamic>? ?? <String, dynamic>{},
      createdAt: DateTime.parse(json['createdAt'] as String),
    );
  }
}

// ---------------------------------------------------------------------------
// Template models
// ---------------------------------------------------------------------------

class TemplateRecord {
  TemplateRecord({
    required this.templateId,
    required this.title,
    required this.description,
    required this.bodyTemplate,
    required this.sections,
    this.protocolId,
    required this.tags,
    required this.createdAt,
    required this.updatedAt,
  });

  final String templateId;
  final String title;
  final String description;
  final String bodyTemplate;
  final List<TemplateSection> sections;
  final String? protocolId;
  final List<String> tags;
  final DateTime createdAt;
  final DateTime updatedAt;

  factory TemplateRecord.fromJson(Map<String, dynamic> json) {
    return TemplateRecord(
      templateId: json['templateId'] as String,
      title: json['title'] as String,
      description: json['description'] as String? ?? '',
      bodyTemplate: json['bodyTemplate'] as String? ?? '',
      sections: (json['sections'] as List<dynamic>? ?? <dynamic>[])
          .map((s) => TemplateSection.fromJson(s as Map<String, dynamic>))
          .toList(),
      protocolId: json['protocolId'] as String?,
      tags: (json['tags'] as List<dynamic>? ?? <dynamic>[]).cast<String>(),
      createdAt: DateTime.parse(json['createdAt'] as String),
      updatedAt: DateTime.parse(json['updatedAt'] as String),
    );
  }
}

class TemplateSection {
  TemplateSection({
    required this.name,
    required this.placeholder,
    required this.required_,
  });

  final String name;
  final String placeholder;
  final bool required_;

  factory TemplateSection.fromJson(Map<String, dynamic> json) {
    return TemplateSection(
      name: json['name'] as String? ?? '',
      placeholder: json['placeholder'] as String? ?? '',
      required_: json['required'] as bool? ?? false,
    );
  }

  Map<String, dynamic> toJson() => {
        'name': name,
        'placeholder': placeholder,
        'required': required_,
      };
}

// ---------------------------------------------------------------------------
// Preview / Thumbnail models
// ---------------------------------------------------------------------------

class PreviewRecord {
  PreviewRecord({
    required this.previewId,
    required this.attachmentId,
    required this.previewType,
    required this.mimeType,
    required this.width,
    required this.height,
    required this.dataBase64,
    required this.createdAt,
  });

  final String previewId;
  final String attachmentId;
  final String previewType;
  final String mimeType;
  final int width;
  final int height;
  final String dataBase64;
  final DateTime createdAt;

  factory PreviewRecord.fromJson(Map<String, dynamic> json) {
    return PreviewRecord(
      previewId: json['previewId'] as String,
      attachmentId: json['attachmentId'] as String? ?? '',
      previewType: json['previewType'] as String? ?? 'thumbnail',
      mimeType: json['mimeType'] as String? ?? 'image/png',
      width: (json['width'] as num?)?.toInt() ?? 0,
      height: (json['height'] as num?)?.toInt() ?? 0,
      dataBase64: json['dataBase64'] as String? ?? '',
      createdAt: DateTime.parse(json['createdAt'] as String),
    );
  }
}

// ---------------------------------------------------------------------------
// Search models
// ---------------------------------------------------------------------------

class SearchResultRecord {
  SearchResultRecord({
    required this.type,
    required this.id,
    required this.title,
    required this.snippet,
    required this.rank,
    this.status,
  });

  final String type;
  final String id;
  final String title;
  final String snippet;
  final double rank;
  final String? status;

  factory SearchResultRecord.fromJson(Map<String, dynamic> json) {
    return SearchResultRecord(
      type: json['type'] as String? ?? 'experiment',
      id: json['id'] as String,
      title: json['title'] as String? ?? '',
      snippet: json['snippet'] as String? ?? '',
      rank: (json['rank'] as num?)?.toDouble() ?? 0.0,
      status: json['status'] as String?,
    );
  }
}

// ---------------------------------------------------------------------------
// Tag model
// ---------------------------------------------------------------------------

class TagRecord {
  TagRecord({required this.tagId, required this.name});

  final String tagId;
  final String name;

  factory TagRecord.fromJson(Map<String, dynamic> json) {
    return TagRecord(
      tagId: json['tagId'] as String,
      name: json['name'] as String,
    );
  }
}

// ---------------------------------------------------------------------------
// Reagent models — mutable lab inventory
// ---------------------------------------------------------------------------

class ReagentStorage {
  ReagentStorage({
    required this.id,
    required this.name,
    required this.locationType,
    required this.description,
    required this.createdAt,
    required this.updatedAt,
  });

  final int id;
  final String name;
  final String locationType;
  final String description;
  final DateTime createdAt;
  final DateTime updatedAt;

  factory ReagentStorage.fromJson(Map<String, dynamic> json) {
    return ReagentStorage(
      id: (json['id'] as num).toInt(),
      name: json['name'] as String? ?? '',
      locationType: json['locationType'] as String? ?? '',
      description: json['description'] as String? ?? '',
      createdAt: DateTime.parse(json['createdAt'] as String),
      updatedAt: DateTime.parse(json['updatedAt'] as String),
    );
  }

  Map<String, dynamic> toJson() => {
    'name': name,
    'locationType': locationType,
    'description': description,
  };
}

class ReagentBox {
  ReagentBox({
    required this.id,
    required this.boxNo,
    required this.boxType,
    required this.owner,
    required this.label,
    required this.location,
    required this.drawer,
    required this.position,
    this.storageId,
    required this.createdAt,
    required this.updatedAt,
  });

  final int id;
  final String boxNo;
  final String boxType;
  final String owner;
  final String label;
  final String location;
  final String drawer;
  final String position;
  final int? storageId;
  final DateTime createdAt;
  final DateTime updatedAt;

  factory ReagentBox.fromJson(Map<String, dynamic> json) {
    return ReagentBox(
      id: (json['id'] as num).toInt(),
      boxNo: json['boxNo'] as String? ?? '',
      boxType: json['boxType'] as String? ?? '',
      owner: json['owner'] as String? ?? '',
      label: json['label'] as String? ?? '',
      location: json['location'] as String? ?? '',
      drawer: json['drawer'] as String? ?? '',
      position: json['position'] as String? ?? '',
      storageId: (json['storageId'] as num?)?.toInt(),
      createdAt: DateTime.parse(json['createdAt'] as String),
      updatedAt: DateTime.parse(json['updatedAt'] as String),
    );
  }

  Map<String, dynamic> toJson() => {
    'boxNo': boxNo,
    'boxType': boxType,
    'owner': owner,
    'label': label,
    'location': location,
    'drawer': drawer,
    'position': position,
    if (storageId != null) 'storageId': storageId,
  };
}

class ReagentAntibody {
  ReagentAntibody({
    required this.id,
    required this.antibodyName,
    required this.catalogNo,
    required this.company,
    required this.antibodyClass,
    required this.antigen,
    required this.host,
    required this.investigator,
    required this.expId,
    required this.notes,
    this.boxId,
    required this.location,
    required this.quantity,
    required this.isDepleted,
    required this.createdAt,
    required this.updatedAt,
  });

  final int id;
  final String antibodyName;
  final String catalogNo;
  final String company;
  final String antibodyClass;
  final String antigen;
  final String host;
  final String investigator;
  final String expId;
  final String notes;
  final int? boxId;
  final String location;
  final String quantity;
  final bool isDepleted;
  final DateTime createdAt;
  final DateTime updatedAt;

  factory ReagentAntibody.fromJson(Map<String, dynamic> json) {
    return ReagentAntibody(
      id: (json['id'] as num).toInt(),
      antibodyName: json['antibodyName'] as String? ?? '',
      catalogNo: json['catalogNo'] as String? ?? '',
      company: json['company'] as String? ?? '',
      antibodyClass: json['class'] as String? ?? '',
      antigen: json['antigen'] as String? ?? '',
      host: json['host'] as String? ?? '',
      investigator: json['investigator'] as String? ?? '',
      expId: json['expId'] as String? ?? '',
      notes: json['notes'] as String? ?? '',
      boxId: (json['boxId'] as num?)?.toInt(),
      location: json['location'] as String? ?? '',
      quantity: json['quantity'] as String? ?? '',
      isDepleted: json['isDepleted'] as bool? ?? false,
      createdAt: DateTime.parse(json['createdAt'] as String),
      updatedAt: DateTime.parse(json['updatedAt'] as String),
    );
  }

  Map<String, dynamic> toJson() => {
    'antibodyName': antibodyName,
    'catalogNo': catalogNo,
    'company': company,
    'class': antibodyClass,
    'antigen': antigen,
    'host': host,
    'investigator': investigator,
    'expId': expId,
    'notes': notes,
    if (boxId != null) 'boxId': boxId,
    'location': location,
    'quantity': quantity,
    'isDepleted': isDepleted,
  };
}

class ReagentCellLine {
  ReagentCellLine({
    required this.id,
    required this.cellLineName,
    required this.selection,
    required this.species,
    required this.parentalCell,
    required this.medium,
    required this.obtainFrom,
    required this.investigator,
    required this.notes,
    this.boxId,
    required this.location,
    required this.quantity,
    required this.isDepleted,
    required this.createdAt,
    required this.updatedAt,
  });

  final int id;
  final String cellLineName;
  final String selection;
  final String species;
  final String parentalCell;
  final String medium;
  final String obtainFrom;
  final String investigator;
  final String notes;
  final int? boxId;
  final String location;
  final String quantity;
  final bool isDepleted;
  final DateTime createdAt;
  final DateTime updatedAt;

  factory ReagentCellLine.fromJson(Map<String, dynamic> json) {
    return ReagentCellLine(
      id: (json['id'] as num).toInt(),
      cellLineName: json['cellLineName'] as String? ?? '',
      selection: json['selection'] as String? ?? '',
      species: json['species'] as String? ?? '',
      parentalCell: json['parentalCell'] as String? ?? '',
      medium: json['medium'] as String? ?? '',
      obtainFrom: json['obtainFrom'] as String? ?? '',
      investigator: json['investigator'] as String? ?? '',
      notes: json['notes'] as String? ?? '',
      boxId: (json['boxId'] as num?)?.toInt(),
      location: json['location'] as String? ?? '',
      quantity: json['quantity'] as String? ?? '',
      isDepleted: json['isDepleted'] as bool? ?? false,
      createdAt: DateTime.parse(json['createdAt'] as String),
      updatedAt: DateTime.parse(json['updatedAt'] as String),
    );
  }

  Map<String, dynamic> toJson() => {
    'cellLineName': cellLineName,
    'selection': selection,
    'species': species,
    'parentalCell': parentalCell,
    'medium': medium,
    'obtainFrom': obtainFrom,
    'investigator': investigator,
    'notes': notes,
    if (boxId != null) 'boxId': boxId,
    'location': location,
    'quantity': quantity,
    'isDepleted': isDepleted,
  };
}

class ReagentVirus {
  ReagentVirus({
    required this.id,
    required this.virusName,
    required this.backbone,
    required this.insertGene,
    required this.envelopeGene,
    required this.investigator,
    required this.notes,
    this.boxId,
    required this.location,
    required this.quantity,
    required this.isDepleted,
    required this.createdAt,
    required this.updatedAt,
  });

  final int id;
  final String virusName;
  final String backbone;
  final String insertGene;
  final String envelopeGene;
  final String investigator;
  final String notes;
  final int? boxId;
  final String location;
  final String quantity;
  final bool isDepleted;
  final DateTime createdAt;
  final DateTime updatedAt;

  factory ReagentVirus.fromJson(Map<String, dynamic> json) {
    return ReagentVirus(
      id: (json['id'] as num).toInt(),
      virusName: json['virusName'] as String? ?? '',
      backbone: json['backbone'] as String? ?? '',
      insertGene: json['insertGene'] as String? ?? '',
      envelopeGene: json['envelopeGene'] as String? ?? '',
      investigator: json['investigator'] as String? ?? '',
      notes: json['notes'] as String? ?? '',
      boxId: (json['boxId'] as num?)?.toInt(),
      location: json['location'] as String? ?? '',
      quantity: json['quantity'] as String? ?? '',
      isDepleted: json['isDepleted'] as bool? ?? false,
      createdAt: DateTime.parse(json['createdAt'] as String),
      updatedAt: DateTime.parse(json['updatedAt'] as String),
    );
  }

  Map<String, dynamic> toJson() => {
    'virusName': virusName,
    'backbone': backbone,
    'insertGene': insertGene,
    'envelopeGene': envelopeGene,
    'investigator': investigator,
    'notes': notes,
    if (boxId != null) 'boxId': boxId,
    'location': location,
    'quantity': quantity,
    'isDepleted': isDepleted,
  };
}

class ReagentDNA {
  ReagentDNA({
    required this.id,
    required this.dnaName,
    required this.backbone,
    required this.insertGene,
    required this.selection,
    required this.investigator,
    required this.notes,
    this.boxId,
    required this.location,
    required this.quantity,
    required this.isDepleted,
    required this.createdAt,
    required this.updatedAt,
  });

  final int id;
  final String dnaName;
  final String backbone;
  final String insertGene;
  final String selection;
  final String investigator;
  final String notes;
  final int? boxId;
  final String location;
  final String quantity;
  final bool isDepleted;
  final DateTime createdAt;
  final DateTime updatedAt;

  factory ReagentDNA.fromJson(Map<String, dynamic> json) {
    return ReagentDNA(
      id: (json['id'] as num).toInt(),
      dnaName: json['dnaName'] as String? ?? '',
      backbone: json['backbone'] as String? ?? '',
      insertGene: json['insertGene'] as String? ?? '',
      selection: json['selection'] as String? ?? '',
      investigator: json['investigator'] as String? ?? '',
      notes: json['notes'] as String? ?? '',
      boxId: (json['boxId'] as num?)?.toInt(),
      location: json['location'] as String? ?? '',
      quantity: json['quantity'] as String? ?? '',
      isDepleted: json['isDepleted'] as bool? ?? false,
      createdAt: DateTime.parse(json['createdAt'] as String),
      updatedAt: DateTime.parse(json['updatedAt'] as String),
    );
  }

  Map<String, dynamic> toJson() => {
    'dnaName': dnaName,
    'backbone': backbone,
    'insertGene': insertGene,
    'selection': selection,
    'investigator': investigator,
    'notes': notes,
    if (boxId != null) 'boxId': boxId,
    'location': location,
    'quantity': quantity,
    'isDepleted': isDepleted,
  };
}

class ReagentOligo {
  ReagentOligo({
    required this.id,
    required this.oligoName,
    required this.sequence,
    required this.length,
    required this.purification,
    required this.investigator,
    required this.notes,
    this.boxId,
    required this.location,
    required this.quantity,
    required this.isDepleted,
    required this.createdAt,
    required this.updatedAt,
  });

  final int id;
  final String oligoName;
  final String sequence;
  final int length;
  final String purification;
  final String investigator;
  final String notes;
  final int? boxId;
  final String location;
  final String quantity;
  final bool isDepleted;
  final DateTime createdAt;
  final DateTime updatedAt;

  factory ReagentOligo.fromJson(Map<String, dynamic> json) {
    return ReagentOligo(
      id: (json['id'] as num).toInt(),
      oligoName: json['oligoName'] as String? ?? '',
      sequence: json['sequence'] as String? ?? '',
      length: (json['length'] as num?)?.toInt() ?? 0,
      purification: json['purification'] as String? ?? '',
      investigator: json['investigator'] as String? ?? '',
      notes: json['notes'] as String? ?? '',
      boxId: (json['boxId'] as num?)?.toInt(),
      location: json['location'] as String? ?? '',
      quantity: json['quantity'] as String? ?? '',
      isDepleted: json['isDepleted'] as bool? ?? false,
      createdAt: DateTime.parse(json['createdAt'] as String),
      updatedAt: DateTime.parse(json['updatedAt'] as String),
    );
  }

  Map<String, dynamic> toJson() => {
    'oligoName': oligoName,
    'sequence': sequence,
    'length': length,
    'purification': purification,
    'investigator': investigator,
    'notes': notes,
    if (boxId != null) 'boxId': boxId,
    'location': location,
    'quantity': quantity,
    'isDepleted': isDepleted,
  };
}

class ReagentChemical {
  ReagentChemical({
    required this.id,
    required this.chemicalName,
    required this.casNumber,
    required this.formula,
    required this.mw,
    required this.company,
    required this.catalogNo,
    required this.concentration,
    required this.investigator,
    required this.notes,
    this.boxId,
    required this.location,
    required this.quantity,
    required this.isDepleted,
    required this.createdAt,
    required this.updatedAt,
  });

  final int id;
  final String chemicalName;
  final String casNumber;
  final String formula;
  final String mw;
  final String company;
  final String catalogNo;
  final String concentration;
  final String investigator;
  final String notes;
  final int? boxId;
  final String location;
  final String quantity;
  final bool isDepleted;
  final DateTime createdAt;
  final DateTime updatedAt;

  factory ReagentChemical.fromJson(Map<String, dynamic> json) {
    return ReagentChemical(
      id: (json['id'] as num).toInt(),
      chemicalName: json['chemicalName'] as String? ?? '',
      casNumber: json['casNumber'] as String? ?? '',
      formula: json['formula'] as String? ?? '',
      mw: json['mw'] as String? ?? '',
      company: json['company'] as String? ?? '',
      catalogNo: json['catalogNo'] as String? ?? '',
      concentration: json['concentration'] as String? ?? '',
      investigator: json['investigator'] as String? ?? '',
      notes: json['notes'] as String? ?? '',
      boxId: (json['boxId'] as num?)?.toInt(),
      location: json['location'] as String? ?? '',
      quantity: json['quantity'] as String? ?? '',
      isDepleted: json['isDepleted'] as bool? ?? false,
      createdAt: DateTime.parse(json['createdAt'] as String),
      updatedAt: DateTime.parse(json['updatedAt'] as String),
    );
  }

  Map<String, dynamic> toJson() => {
    'chemicalName': chemicalName,
    'casNumber': casNumber,
    'formula': formula,
    'mw': mw,
    'company': company,
    'catalogNo': catalogNo,
    'concentration': concentration,
    'investigator': investigator,
    'notes': notes,
    if (boxId != null) 'boxId': boxId,
    'location': location,
    'quantity': quantity,
    'isDepleted': isDepleted,
  };
}

class ReagentMolecular {
  ReagentMolecular({
    required this.id,
    required this.reagentName,
    required this.catalogNo,
    required this.company,
    required this.lotNo,
    required this.reagentType,
    required this.investigator,
    required this.notes,
    this.boxId,
    required this.location,
    required this.quantity,
    required this.isDepleted,
    required this.createdAt,
    required this.updatedAt,
  });

  final int id;
  final String reagentName;
  final String catalogNo;
  final String company;
  final String lotNo;
  final String reagentType;
  final String investigator;
  final String notes;
  final int? boxId;
  final String location;
  final String quantity;
  final bool isDepleted;
  final DateTime createdAt;
  final DateTime updatedAt;

  factory ReagentMolecular.fromJson(Map<String, dynamic> json) {
    return ReagentMolecular(
      id: (json['id'] as num).toInt(),
      reagentName: json['reagentName'] as String? ?? '',
      catalogNo: json['catalogNo'] as String? ?? '',
      company: json['company'] as String? ?? '',
      lotNo: json['lotNo'] as String? ?? '',
      reagentType: json['reagentType'] as String? ?? '',
      investigator: json['investigator'] as String? ?? '',
      notes: json['notes'] as String? ?? '',
      boxId: (json['boxId'] as num?)?.toInt(),
      location: json['location'] as String? ?? '',
      quantity: json['quantity'] as String? ?? '',
      isDepleted: json['isDepleted'] as bool? ?? false,
      createdAt: DateTime.parse(json['createdAt'] as String),
      updatedAt: DateTime.parse(json['updatedAt'] as String),
    );
  }

  Map<String, dynamic> toJson() => {
    'reagentName': reagentName,
    'catalogNo': catalogNo,
    'company': company,
    'lotNo': lotNo,
    'reagentType': reagentType,
    'investigator': investigator,
    'notes': notes,
    if (boxId != null) 'boxId': boxId,
    'location': location,
    'quantity': quantity,
    'isDepleted': isDepleted,
  };
}

class ReagentSearchResult {
  ReagentSearchResult({
    required this.type,
    required this.id,
    required this.name,
  });

  final String type;
  final int id;
  final String name;

  factory ReagentSearchResult.fromJson(Map<String, dynamic> json) {
    return ReagentSearchResult(
      type: json['type'] as String,
      id: (json['id'] as num).toInt(),
      name: json['name'] as String,
    );
  }
}
